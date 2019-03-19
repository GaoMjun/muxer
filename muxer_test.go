package muxer

import (
	"io"
	"log"
	"net"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lshortfile)
}

func TestMuxer(t *testing.T) {
	go server()

	time.Sleep(time.Second * 3)

	client()
}

func client() {
	var (
		err   error
		conn  net.Conn
		muxer *Muxer
	)
	defer func() {
		if err != nil {
			log.Println(err)
		}
		if conn != nil {
			conn.Close()
		}
	}()

	if conn, err = net.Dial("tcp", "127.0.0.1:12345"); err != nil {
		return
	}

	muxer = New(conn)

	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)
	go openstream(muxer)

	select {}
}

func openstream(muxer *Muxer) {
	var (
		err    error
		stream *Stream
	)
	defer func() {
		if err != nil {
			log.Println(err)
		}
	}()

	if stream, err = muxer.OpenStream(); err != nil {
		return
	}

	go func() {
		var (
			err    error
			buffer = make([]byte, 1024)
			n      = 0
		)
		defer func() {
			if err != nil {
				log.Println(err)
			}
		}()

		for {
			if n, err = stream.Read(buffer); err != nil {
				return
			}

			log.Println("server: #stream", stream.ID, string(buffer[:n]))
			n = n
		}
	}()

	go func() {
		time.Sleep(time.Second * 10)
		stream.Close()
	}()

	for {
		if _, err = stream.Write([]byte("ping")); err != nil {
			break
		}

		log.Println("client: #stream", stream.ID, "ping")

		time.Sleep(time.Millisecond * 500)
	}
}

func server() {
	var (
		err error
		l   net.Listener
	)
	defer func() {
		if err != nil {
			log.Println(err)
		}
	}()

	if l, err = net.Listen("tcp", "127.0.0.1:12345"); err != nil {
		return
	}

	for {
		conn, _ := l.Accept()
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	var (
		err    error
		muxer  = New(conn)
		stream *Stream
	)
	defer func() {
		conn.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	for {
		if stream, err = muxer.AcceptStream(); err != nil {
			return
		}

		go handleStream(stream)
	}
}

func handleStream(stream *Stream) {
	// var (
	// 	err    error
	// 	buffer = make([]byte, 1024)
	// 	n      = 0
	// )
	// defer func() {
	// 	if err != nil {
	// 		log.Println(err)
	// 	}
	// }()

	// for {
	// 	if n, err = stream.Read(buffer); err != nil {
	// 		return
	// 	}

	// 	log.Println("server: #stream", stream.ID, string(buffer[:n]))
	// }

	io.Copy(stream, stream)
	stream.Close()
}
