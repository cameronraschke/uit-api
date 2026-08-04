package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type LogBuffer struct {
	Mu       sync.RWMutex
	Buffer   *bytes.Buffer
	Capacity int
	Output   io.Writer
}

func newLogBuffer(output io.Writer, capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = 64 * 1024 // Default to 64KB if capacity is not specified
	}
	return &LogBuffer{
		Mu:       sync.RWMutex{},
		Buffer:   bytes.NewBuffer(make([]byte, 0, capacity)),
		Capacity: capacity,
		Output:   output,
	}
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	if lb == nil || lb.Buffer == nil || lb.Buffer.Len() == 0 {
		return 0, fmt.Errorf("log buffer is nil")
	}
	if lb.Capacity <= 0 {
		lb.Capacity = 64 * 1024
	}

	lb.Mu.Lock()
	defer lb.Mu.Unlock()

	if len(p) >= lb.Capacity {
		p = p[len(p)-lb.Capacity:]
		lb.Buffer.Reset()
	} else if lb.Buffer.Len()+len(p) > lb.Capacity {
		// If the buffer exceeds capacity, truncate the oldest data.
		excess := lb.Buffer.Len() + len(p) - lb.Capacity
		if excess >= lb.Buffer.Len() {
			lb.Buffer.Reset()
		} else {
			lb.Buffer.Next(excess)
		}
	}
	return lb.Buffer.Write(p)
}

func (lb *LogBuffer) Read(p []byte) (n int, err error) {
	if lb == nil || lb.Buffer == nil || lb.Buffer.Len() == 0 {
		return 0, fmt.Errorf("log buffer is nil")
	}
	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	return lb.Buffer.Read(p)
}

func (lb *LogBuffer) String() string {
	if lb == nil || lb.Buffer == nil || lb.Buffer.Len() == 0 {
		return ""
	}
	lb.Mu.RLock()
	defer lb.Mu.RUnlock()
	return lb.Buffer.String()
}

func (lb *LogBuffer) Clear() {
	if lb == nil || lb.Buffer == nil || lb.Buffer.Len() == 0 {
		return
	}
	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	lb.Buffer.Reset()
}

func (lb *LogBuffer) Flush() error {
	if lb == nil || lb.Buffer == nil || lb.Buffer.Len() == 0 {
		return fmt.Errorf("log buffer is nil")
	}
	if lb.Output == nil {
		return fmt.Errorf("log buffer output is nil")
	}
	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	data := lb.Buffer.Bytes()
	if len(data) == 0 {
		return nil
	}
	written, err := lb.Output.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return fmt.Errorf("short write while flushing log buffer: wrote %d of %d bytes", written, len(data))
	}
	lb.Buffer.Reset()
	return nil
}

var (
	StdoutBuffer atomic.Pointer[LogBuffer]
	StderrBuffer atomic.Pointer[LogBuffer]
)

func (logBuffer *LogBuffer) addLogToBuffer(msg string) {
	if logBuffer == nil || msg == "" {
		return
	}
	_, _ = logBuffer.Write([]byte(msg))
	if len(msg) == 0 || msg[len(msg)-1] != '\n' {
		_, _ = logBuffer.Write([]byte("\n"))
	}
}

func FlushLogBuffers() error {
	stdoutBuffer := StdoutBuffer.Load()
	if stdoutBuffer != nil {
		if err := stdoutBuffer.Flush(); err != nil {
			return fmt.Errorf("failed to flush stdout log buffer: %w", err)
		}
	}

	stderrBuffer := StderrBuffer.Load()
	if stderrBuffer != nil {
		if err := stderrBuffer.Flush(); err != nil {
			return fmt.Errorf("failed to flush stderr log buffer: %w", err)
		}
	}
	return nil
}

func initLogBuffers() error {
	StdoutBuffer.Store(newLogBuffer(os.Stdout, 64*1024)) // 64KB buffer for stdout
	StderrBuffer.Store(newLogBuffer(os.Stderr, 64*1024)) // 64KB buffer for stderr
	return nil
}
