/*
Simple WebSocket Load Tester
----------------------------
This program will simulate multiple concurrent WebSocket clients
connecting to a chat server and sending periodic messages.
Each client will records metrics like number of messages sent,
messages received, latency, and errors.
*/

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
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
	TotalLatency  int64 // in nanoseconds
}

func client(id int, metrics *Metrics, targetUrl string, testDur time.Duration) {
	conn, _, err := websocket.DefaultDialer.Dial(targetUrl, nil)
	if err != nil {
		atomic.AddInt64(&metrics.NumErrors, 1) // This must be atomic due to concurrency (multiple clients in goroutines)
		return
	}
	defer func() {
		// Send a close message to avoid `websocket: close 1006 (abnormal closure): unexpected EOF`
		// Wait a bit (200ms) to ensure the message is sent before closing the connection
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "test complete"))
		time.Sleep(200 * time.Millisecond)
		conn.Close()
	}()

	// Listen for interrupt signals to close the connection properly (is this necessary?)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Increment total connections
	atomic.AddInt64(&metrics.NumTotalConns, 1)

	// Buffered channel to read broadcasted messages
	// (Size is arbitrary, at least double the number of clients to account for 'new user joined' messages)
	recvCh := make(chan []byte, 200)

	// Continuously read messages broadcasted by server
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				// Don't count normal closure as an error (sending errors is how WebSocket terminates apparently?)
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				atomic.AddInt64(&metrics.NumErrors, 1)
				return
			}
			atomic.AddInt64(&metrics.NumMsgsRecv, 1)
			recvCh <- msg
		}
	}()

	// Record number of messages sent and total latency for this client
	nSent := int64(0)
	clientTotalLatency := int64(0) // in nanoseconds

	// For the duration of the test, send a message every 2 seconds, record metrics
	start := time.Now()
	for time.Since(start) < testDur {
		msg := []byte(fmt.Sprint("Hello from client ", id))
		sendTime := time.Now().UnixNano()
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			fmt.Println("Write error:", err)
			atomic.AddInt64(&metrics.NumErrors, 1)
			return
		}
		atomic.AddInt64(&metrics.NumMsgsSent, 1)
		nSent++

		select {
		case reply := <-recvCh:
			// Only record latency if it's the echoed message
			if bytes.Equal(reply, msg) {
				latency := time.Now().UnixNano() - sendTime
				clientTotalLatency += latency
			}
		case <-time.After(1 * time.Second):
			fmt.Println("Timeout waiting for reply")
			atomic.AddInt64(&metrics.NumErrors, 1)
		}

		time.Sleep(2 * time.Second) // Waiting a bit before sending the next message
	}

	// Calculate average latency for this client and add to total metrics
	if nSent > 0 {
		avgLatency := clientTotalLatency / nSent
		atomic.AddInt64(&metrics.TotalLatency, avgLatency)
		fmt.Println("Client ", id, " avg latency: ", avgLatency, "ns")
	}
}

func main() {
	// Setup test parameters
	testDur := 10 * time.Second
	nClients := 100
	targetUrl := "ws://localhost:8080/ws"
	metrics := &Metrics{
		NumTotalConns: 0,
		NumMsgsSent:   0,
		NumMsgsRecv:   0,
		NumErrors:     0,
	}

	// Launch each client in a goroutine and wait for all of them to finish execution
	var wg sync.WaitGroup
	for i := range nClients {
		wg.Go(func() {
			client(i, metrics, targetUrl, testDur)
		})
	}
	wg.Wait()

	// Print metrics
	fmt.Println("Total number of connections: ", metrics.NumTotalConns)
	fmt.Println("Number of messages sent: ", metrics.NumMsgsSent)
	fmt.Println("Number of messages received: ", metrics.NumMsgsRecv)
	fmt.Println("Number of errors encountered: ", metrics.NumErrors)
	if metrics.NumMsgsRecv > 0 {
		fmt.Println("Average latency: ", metrics.TotalLatency/metrics.NumTotalConns, "ns")
	}
}
