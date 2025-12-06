package middleware

import "bot-go-template/internal/adapter/telegram"

// Middleware wraps telegram.HandlerFunc.
type Middleware func(telegram.HandlerFunc) telegram.HandlerFunc

// Chain applies middlewares in order.
func Chain(h telegram.HandlerFunc, mws ...Middleware) telegram.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
