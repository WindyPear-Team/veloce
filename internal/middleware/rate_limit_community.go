package middleware

import "github.com/gin-gonic/gin"

var rateLimiterFactory func() gin.HandlerFunc

type RateLimiter struct {
	handler gin.HandlerFunc
}

func RegisterRateLimiterFactory(factory func() gin.HandlerFunc) {
	rateLimiterFactory = factory
}

func NewRateLimiter() *RateLimiter {
	if rateLimiterFactory != nil {
		return &RateLimiter{handler: rateLimiterFactory()}
	}
	return &RateLimiter{}
}

func (limiter *RateLimiter) Middleware() gin.HandlerFunc {
	if limiter.handler != nil {
		return limiter.handler
	}
	return func(c *gin.Context) {
		c.Next()
	}
}
