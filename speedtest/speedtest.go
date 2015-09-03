package speedtest

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/dustin/go-humanize"
)

type Stats struct {
	Bytes, Blocks int64
	Elapsed       time.Duration
	Overhead      time.Duration
	// data source didn't have a new block ready, previous buffer was reused.
	Repeats int64
	// data sink couldn't receive read data, block dropped
	Dropped int64
}

func Provide(c net.Conn, dataSource <-chan []byte, doneChunks chan<- []byte, stats chan<- Stats) error {
	data := <-dataSource

	var s Stats
	start := time.Now()
	overhead := start
	for {
		s.Overhead += time.Since(overhead)
		n, err := c.Write(data)
		overhead = time.Now()

		if err != nil {
			return err
		}
		s.Bytes += int64(n)
		s.Blocks++
		s.Elapsed = time.Since(start)

		select {
		case stats <- s:
		default:
		}

		select {
		case doneChunks <- data:
		default:
			s.Dropped++
		}

		select {
		case newData, open := <-dataSource:
			if !open {
				return io.EOF
			}
			data = newData
		default:
			s.Repeats++
		}
	}
}

func Consume(c net.Conn, blockSource <-chan []byte, blockSink chan<- []byte, stats chan<- Stats) error {
	buf := <-blockSource

	var s Stats
	start := time.Now()
	overhead := start
	for {
		s.Overhead += time.Since(overhead)
		n, err := c.Read(buf)
		overhead = time.Now()

		if err != nil {
			return err
		}
		s.Bytes += int64(n)
		s.Blocks++
		s.Elapsed = time.Since(start)

		select {
		case stats <- s:
		default:
		}

		select {
		case blockSink <- buf:
		default:
			s.Dropped++
		}

		select {
		case newBuf, open := <-blockSource:
			if !open {
				return io.EOF
			}
			buf = newBuf
		default:
			s.Repeats++
		}
	}
}

func Report(prefix string, rate time.Duration, stats <-chan Stats) {
	for _ = range time.NewTicker(rate).C {
		s, open := <-stats
		if !open {
			return
		}
		if s.Elapsed == 0 {
			continue
		}
		Bps := uint64(float64(s.Bytes) / (s.Elapsed.Seconds()))
		fmt.Printf("%s: %s in %s: %sps  [%s, %d repeats, %d drops, %v overhead]\n",
			prefix,
			humanize.Bytes(uint64(s.Bytes)), s.Elapsed, humanize.Bytes(Bps),
			humanize.SI(float64(s.Blocks), " blocks"),
			s.Repeats, s.Dropped, s.Overhead)
	}
}
