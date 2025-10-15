package main

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Metrics struct {
	NumTotalConns int64
	NumMsgsSent   int64
	NumMsgsRecv   int64
	NumErrors     int64
	TotalLatency  int64 // nanoseconds
}

func startClient(id int, metrics *Metrics, targetUrl string, testDur time.Duration) {
	conn, _, err := websocket.DefaultDialer.Dial(targetUrl, nil)
	if err != nil {
		atomic.AddInt64(&metrics.NumErrors, 1)
		return
	}

	// Record number of messages sent and total latency for this client
	nSent := int64(0)
	clientTotalLatency := int64(0) // in nanoseconds

	defer func() {
		// Record latency metrics
		if nSent > 0 {
			// avg := float64(clientTotalLatency) / float64(nSent)
			// fmt.Printf("Client #%d avg latency: %.2f ms\n", id, avg/1e6)
			atomic.AddInt64(&metrics.TotalLatency, clientTotalLatency)
		}

		// On return: send a close message to avoid `websocket: close 1006 (abnormal closure): unexpected EOF`
		// Wait a bit (200ms) to ensure the message is sent before closing the connection
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("client %d done", id)))
		time.Sleep(200 * time.Millisecond)
		conn.Close()
	}()

	atomic.AddInt64(&metrics.NumTotalConns, 1)

	recvCh := make(chan []byte, 256) // Buffered channel to contain broadcasted messages
	done := make(chan struct{})      // Unbuffered channel to signal to the main loop to stop

	// Reader goroutine
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					fmt.Printf("client %d read error: %v\n", id, err)
					atomic.AddInt64(&metrics.NumErrors, 1)
				}
				break
			}
			atomic.AddInt64(&metrics.NumMsgsRecv, 1)

			// Non-blocking send to recvCh
			// If we use `recvCh <- msg` outside `select`, it will block when `recvCh` is full,
			// making the test client a bottleneck for load-testing
			select {
			case recvCh <- msg:
			default:
			}
		}
	}()

	// For the duration of the test, send a message every 2 seconds, record metrics
	start := time.Now()
	for time.Since(start) < testDur {
		msg := []byte(fmt.Sprintf("Hello from client %d", id))
		sendTime := time.Now()

		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			fmt.Printf("client %d write error: %v\n", id, err)
			atomic.AddInt64(&metrics.NumErrors, 1)
			break
		}

		atomic.AddInt64(&metrics.NumMsgsSent, 1)
		nSent++

		select {
		case reply := <-recvCh:
			if bytes.Equal(reply, msg) {
				latency := time.Since(sendTime)
				clientTotalLatency += latency.Nanoseconds()
			}
		case <-time.After(1 * time.Second):
			fmt.Println("Timeout waiting for reply")
			atomic.AddInt64(&metrics.NumErrors, 1)
		case <-done:
			return
		}

		time.Sleep(2 * time.Second)
	}
}

func main() {
	// Setup test parameters
	testDur := 30 * time.Second
	nClients := 1000
	targetUrl := "ws://localhost:8080/ws"

	// Launch each client in a goroutine and wait for all of them to finish execution
	metrics := &Metrics{}
	var wg sync.WaitGroup

	for i := 0; i < nClients; i++ {
		wg.Go(func() {
			startClient(i, metrics, targetUrl, testDur)
		})
		time.Sleep(200 * time.Millisecond) // ramp-up
	}

	wg.Wait()

	// Print metrics
	fmt.Println("==== Load Test Summary ====")
	fmt.Println("Total clients:", metrics.NumTotalConns)
	fmt.Println("Messages sent: ", metrics.NumMsgsSent)
	fmt.Println("Messages received: ", metrics.NumMsgsRecv)
	fmt.Println("Errors: ", metrics.NumErrors)

	if metrics.NumMsgsRecv > 0 {
		avgLatency := float64(metrics.TotalLatency) / float64(metrics.NumMsgsRecv)
		fmt.Printf("Average latency: %.2f ms\n", avgLatency/1e6)
	}
}
