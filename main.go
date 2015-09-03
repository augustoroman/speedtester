package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/augustoroman/speedtester/speedtest"
	"github.com/dustin/go-humanize"
	"gopkg.in/alecthomas/kingpin.v2"
)

func randomize(b []byte) {
	logrus.Printf("Randomizing %s buffer", humanize.IBytes(uint64(len(b))))
	rand.Read(b)
}

func main() {
	download := kingpin.Command("download", "Check download speed from server.")
	downloadServer := download.Arg("server", "Server address, including port").String()

	upload := kingpin.Command("upload", "Check upload speed from server.")
	uploadServer := upload.Arg("server", "Server address, including port").String()

	serve := kingpin.Command("serve", "Start a server that clients can connect to.")
	serveAddr := serve.Flag("addr", "Address to serve on.").Default(":5555").String()

	chunkSize := kingpin.Flag("chunk-size", "Size of each buffer chunk").Short('c').Default("64KB").Bytes()
	bufferSize := kingpin.Flag("buffer-size", "Total buffer size").Short('s').Default("16MiB").Bytes()

	cmd := kingpin.Parse()

	megaBuffer := make([]byte, *bufferSize)

	numChunks := int(*bufferSize / *chunkSize)
	buffers := make(chan []byte, numChunks+10)
	for i := 0; i < numChunks; i++ {
		buffers <- megaBuffer[int(*chunkSize)*i : int(*chunkSize)*(i+1)]
	}

	switch cmd {
	case serve.FullCommand():
		randomize(megaBuffer)
		(server{buffers, *serveAddr}).run()
	case upload.FullCommand():
		randomize(megaBuffer)
		doUpload(buffers, *uploadServer)
	case download.FullCommand():
		doDownload(buffers, *downloadServer)
	}
}

func doUpload(buffers chan []byte, addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		logrus.Fatalf("Cannot connect to %v: %v", addr, err)
	}
	logrus.Printf("Connected to %s: Uploading", addr)

	io.WriteString(conn, "upload")

	stats := newLogger(fmt.Sprintf("Server %v", addr))
	if err := speedtest.Provide(conn, buffers, buffers, stats); err != nil {
		logrus.Fatal(err)
	}
	stats <- speedtest.Stats{} // flush the last entry.
	close(stats)
	logrus.Print("Done")
}

func doDownload(buffers chan []byte, addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		logrus.Fatalf("Cannot connect to %v: %v", addr, err)
	}
	logrus.Printf("Connected to %s: Downloading", addr)

	io.WriteString(conn, "download")

	stats := newLogger(fmt.Sprintf("Server %v", addr))
	if err := speedtest.Consume(conn, buffers, buffers, stats); err != nil {
		logrus.Fatal(err)
	}
	stats <- speedtest.Stats{} // flush the last entry.
	close(stats)
	logrus.Print("Done")
}

type server struct {
	buffers chan []byte
	addr    string
}

func (s server) run() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Printf("Listening on %s", s.addr)
	for {
		conn, err := l.Accept()
		if err != nil {
			logrus.Errorf("Connection failure: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s server) handleConn(c net.Conn) {
	client := fmt.Sprintf("Client %v", c.RemoteAddr())
	logrus.Printf("%s: Connected", client)

	defer c.Close()

	stats := newLogger(client)
	defer close(stats)

	// for {
	var op [256]byte
	n, err := c.Read(op[:])
	if err != nil {
		logrus.Errorf("%s: failure: %v", client, err)
		return
	}

	switch string(op[:n]) {
	case "bye":
		logrus.Printf("%s: goodbye", client)
		return
	case "upload":
		if err := speedtest.Consume(c, s.buffers, s.buffers, stats); err != nil {
			logrus.Errorf("%s: failed upload: %v", client, err)
			return
		}
	case "download":
		if err := speedtest.Provide(c, s.buffers, s.buffers, stats); err != nil {
			logrus.Errorf("%s: failed download: %v", client, err)
			return
		}
	}
	// }
}

func newLogger(prefix string) chan speedtest.Stats {
	stats := make(chan speedtest.Stats) // unbuffered
	go speedtest.Report(prefix, time.Second, stats)
	return stats
}
