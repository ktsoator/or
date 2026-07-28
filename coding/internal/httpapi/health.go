package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) mountHealth(r gin.IRouter) {
	r.GET("/health", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNoContent)
	})
}
