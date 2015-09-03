package speedtest

import (
	"fmt"
	"io"
	"net"
	"strings"
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

func (s Stats) Since(s2 Stats) Stats {
	return Stats{
		Bytes:    s.Bytes - s2.Bytes,
		Blocks:   s.Blocks - s2.Blocks,
		Elapsed:  s.Elapsed - s2.Elapsed,
		Overhead: s.Overhead - s2.Overhead,
		Repeats:  s.Repeats - s2.Repeats,
		Dropped:  s.Dropped - s2.Dropped,
	}
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
	var last Stats
	for _ = range time.NewTicker(rate).C {
		s, open := <-stats
		if !open {
			return
		}
		if s.Elapsed == 0 {
			continue
		}
		last, s = s, s.Since(last)
		Bps := uint64(float64(s.Bytes) / (s.Elapsed.Seconds()))
		fmt.Printf("%s: %s in %s: %sbps  [%s, %d repeats, %d drops, %v overhead]\n",
			prefix,
			humanize.Bytes(uint64(s.Bytes)),
			s.Elapsed,
			strings.TrimSuffix(humanize.Bytes(8*Bps), "B"),
			humanize.SI(float64(s.Blocks), " blocks"),
			s.Repeats, s.Dropped, s.Overhead)
	}
}
