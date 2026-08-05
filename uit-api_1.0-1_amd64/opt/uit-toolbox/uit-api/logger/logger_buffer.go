package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type LogBuffer struct {
	Mu       sync.RWMutex
	Buffer   *bufio.Writer
	Capacity int
	Output   io.Writer
}

const (
	logBufferSize = 64 * 1024 // 64KB default buffer size
)

func newLogBuffer(output io.Writer, capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = logBufferSize // Use default buffer size if capacity is not specified
	}
	if output == nil {
		output = io.Discard
	}
	return &LogBuffer{
		Mu:       sync.RWMutex{},
		Buffer:   bufio.NewWriterSize(output, capacity),
		Capacity: capacity,
		Output:   output,
	}
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	if lb == nil || lb.Buffer == nil {
		return 0, fmt.Errorf("log buffer is nil")
	}

	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	return lb.Buffer.Write(p)
}

func (lb *LogBuffer) WriteString(s string) (n int, err error) {
	if lb == nil || lb.Buffer == nil {
		return 0, fmt.Errorf("log buffer is nil")
	}

	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	return lb.Buffer.WriteString(s)
}

func (lb *LogBuffer) Clear() {
	if lb == nil || lb.Buffer == nil {
		return
	}
	if lb.Output == nil {
		return
	}
	lb.Mu.Lock()
	defer lb.Mu.Unlock()
	lb.Buffer.Reset(lb.Output)
}

func (lb *LogBuffer) Flush() error {
	if lb == nil || lb.Buffer == nil {
		return fmt.Errorf("log buffer is nil")
	}
	if lb.Output == nil {
		return fmt.Errorf("log buffer output is nil")
	}
	lb.Mu.Lock()
	defer lb.Mu.Unlock()

	if lb.Buffer.Buffered() == 0 {
		return nil
	}
	return lb.Buffer.Flush()
}

var (
	StdoutBuffer atomic.Pointer[LogBuffer]
	StderrBuffer atomic.Pointer[LogBuffer]
)

func (logBuffer *LogBuffer) addLogToBuffer(msg string) {
	if logBuffer == nil || msg == "" {
		return
	}
	_, _ = logBuffer.WriteString(msg)
	if len(msg) == 0 || msg[len(msg)-1] != '\n' {
		_, _ = logBuffer.WriteString("\n")
	}
}

func processLogBuffers(logBuffer *LogBuffer) {
	if logBuffer == nil {
		return
	}
	_ = logBuffer.Flush()
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
	StdoutBuffer.Store(newLogBuffer(os.Stdout, logBufferSize)) // buffer for stdout
	StderrBuffer.Store(newLogBuffer(os.Stderr, logBufferSize)) // buffer for stderr
	return nil
}
