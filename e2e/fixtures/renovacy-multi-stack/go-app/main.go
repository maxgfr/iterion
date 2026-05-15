// Package main exists only so secured-renovacy's align_code node has a
// real Go call-site referencing a pinned vulnerable gin version
// (CVE-2023-26125). Not production code.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/echo", func(c *gin.Context) {
		c.String(http.StatusOK, c.Query("msg"))
	})
	_ = r.Run(":8080")
}
