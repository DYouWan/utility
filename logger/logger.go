package logger

import (
	"errors"
	"fmt"
	"github.com/dyouwan/utility/file"
	"github.com/dyouwan/utility/pool"
	"os"
	"runtime"
	"time"
)

// DefaultLog 默认的Log实例
var DefaultLog = new(Logger)

// Logger 日志记录
type Logger struct {
	level        Level              // 日志记录器等级
	files        map[Level]*os.File // 日志文件句柄
	inputBuffer  *CircularBuffer    // 环形缓冲区实例,作为一级缓存
	outputBuffer chan *LogMessage   // 通道缓冲区，用于暂存日志消息。 作为二级缓存
}

func init() {
	log, err := NewLogger(DefaultOptions)
	if err != nil {
		panic(err)
	}
	DefaultLog = log
}

// NewLogger 创建一个新的日志记录器实例
func NewLogger(opts Options) (*Logger, error) {
	if opts.Dir == "" {
		return nil, errors.New("invalid log dir")
	}

	if opts.MaxSize <= 0 {
		opts.MaxSize = defaultMaxSize
	}

	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}

	err := file.CrateFile(opts.Dir)
	if err != nil {
		return nil, err
	}

	files := make(map[Level]*os.File)
	for _, level := range AllLevels {
		if level > opts.Level {
			continue
		}

		fileDir := fmt.Sprintf("%s/%s", opts.Dir, level.String())
		err = file.CrateFile(fileDir)
		if err != nil {
			return nil, err
		}

		filePath := fmt.Sprintf("%s/%s/%s.log", opts.Dir, level.String(), time.Now().Format("2006-01-02"))
		fi, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		files[level] = fi
	}

	log := &Logger{
		level:        opts.Level,
		files:        files,
		outputBuffer: make(chan *LogMessage, 1024),
		inputBuffer:  NewCircularBuffer(opts.BufferSize),
	}

	go log.writeBuffer()
	go log.startWorkersOrdered(runtime.NumCPU())

	return log, nil
}

// Debug 记录一条调试信息
func (l *Logger) Debug(msg string, source string) {
	l.Log(DebugLevel, msg, source)
}

// Info 记录一条普通信息
func (l *Logger) Info(msg string, source string) {
	l.Log(InfoLevel, msg, source)
}

// Warning 记录一条警告信息
func (l *Logger) Warning(msg string, source string) {
	l.Log(WarnLevel, msg, source)
}

// Error 记录一条错误信息
func (l *Logger) Error(msg string, source string) {
	l.Log(ErrorLevel, msg, source)
}

// Fatal 记录一条严重错误信息
func (l *Logger) Fatal(msg string, source string) {
	l.Log(FatalLevel, msg, source)
}

// Log 记录一条日志消息
func (l *Logger) Log(level Level, msg string, source string) {
	if l.level <= level {
		logMsg := logMessagePool.Get().(*LogMessage)
		logMsg.level = level
		logMsg.time = time.Now()
		logMsg.msg = msg
		logMsg.source = source
		l.inputBuffer.Write(logMsg)
	}
}

// 后台goroutine，将缓冲区中的消息写入管道中
// 当写入日志文件的操作比较耗时时，后台线程可能会阻塞在写入操作上，无法继续处理其他日志消息，从而导致缓冲区中的消息越来越多，最终导致内存溢出等问题。
// 为了避免这种情况，采用异步写入日志消息的方式。后台线程从缓冲区中读取日志消息后，不再直接将其写入到文件中，而是先将其存储到一个管道（channel）中。
// 然后，另外启动一个或多个协程，负责从管道中读取日志消息，并将其写入到对应的文件中。这样做可以实现异步写入日志消息，避免阻塞后台线程。
func (l *Logger) writeBuffer() {
	for {
		msg := l.inputBuffer.Read()
		if msg != nil {
			l.outputBuffer <- msg
		} else {
			// 如果没有消息，则休眠一段时间
			timer := time.NewTimer(time.Millisecond * 100)
			<-timer.C
		}
	}
}

// 使用 worker pool 处理日志消息。 处理消息时是有序的
func (l *Logger) startWorkersOrdered(num int) {
	workerPool := pool.NewWorkerPool(num)
	workerPool.Start()

	// 将 LogMessage 对象转换为 Job 对象，并提交到 worker pool 中处理
	go func() {
		for {
			select {
			case msg := <-l.outputBuffer:
				job := &LogMessageJob{message: msg}
				workerPool.Submit(job)
			case <-workerPool.Quit:
				return
			}
		}
	}()
}

// 使用 worker pool 处理日志消息。 处理消息时是乱序的
func (l *Logger) startWorkersDisordered(num int) {
	workerPool := pool.NewWorkerPool(num)
	workerPool.Start()

	// 将 LogMessage 对象转换为 Job 对象，并提交到 worker pool 中处理
	for i := 0; i < num; i++ {
		go func() {
			for {
				select {
				case msg := <-l.outputBuffer:
					job := &LogMessageJob{message: msg}
					workerPool.Submit(job)
				case <-workerPool.Quit:
					return
				}
			}
		}()
	}
}

// Info 方法，记录一条 Info 级别的日志
func Info(msg string, source string) {
	DefaultLog.Info(msg, source)
}

// Error 方法，记录一条 Error 级别的日志
func Error(msg string, source string) {
	DefaultLog.Error(msg, source)
}

// Warn 方法，记录一条 Warn 级别的日志
func Warn(msg string, source string) {
	DefaultLog.Warning(msg, source)
}

// Debug 方法，记录一条 Debug 级别的日志
func Debug(msg string, source string) {
	DefaultLog.Debug(msg, source)
}
