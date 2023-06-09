package pipeline

import (
	"net/http"
)

// Pipeline 负责处理http请求管道模型
type Pipeline struct {
	middleware middleware
	handlers   []Handler
}

// New 创建一个管道模型
func New(handlers ...Handler) *Pipeline {
	return &Pipeline{
		handlers:   handlers,
		middleware: build(handlers),
	}
}

// NewPipeline allocates and returns a new Pipeline.
func NewPipeline() *Pipeline { return new(Pipeline) }

// ServeHTTP 实现底层http.Handler接口
func (p *Pipeline) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	p.middleware.ServeHTTP(rw, r)
}

// Use 添加中间件时，会将所有中间件进行编译，形成中间件链。
// 执行Pipeline的ServeHTTP方法时，实际是执行第一个中间件，然后根据中间件的链，依次调用下一个中间件
func (p *Pipeline) Use(handler Handler) {
	if handler == nil {
		panic("handler cannot be nil")
	}

	p.handlers = append(p.handlers, handler)
	p.middleware = build(p.handlers)
}

// UseHandler 添加处理程序
func (p *Pipeline) UseHandler(handler http.Handler) {
	p.Use(Adapt(handler))
}

// Adapt 适配器
func Adapt(h http.Handler) HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		h.ServeHTTP(rw, r)
		next(rw, r)
	}
}

// build 中间件链
func build(handlers []Handler) middleware {
	voidMiddleware := VoidMiddleware()

	switch len(handlers) {
	case 0:
		return VoidMiddleware()
	case 1:
		return middleware{handlers[0], &voidMiddleware}
	case 2:
		return middleware{handlers[0], &middleware{handlers[1], &voidMiddleware}}
	case 3:
		return middleware{handlers[0], &middleware{handlers[1], &middleware{handlers[2], &voidMiddleware}}}
	default:
		var stack []middleware
		for i := len(handlers) - 1; i >= 0; i-- {
			m := middleware{handlers[i], &voidMiddleware}
			if len(stack) > 0 {
				m.next = &stack[len(stack)-1]
			}
			stack = append(stack, m)
		}
		return stack[len(stack)-1]
	}
}
