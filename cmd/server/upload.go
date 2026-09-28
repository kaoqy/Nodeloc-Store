package main

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/platform/upload"
)

// registerUploads gives the shop a way to put a picture on the shelf without
// shell-ing a file into the volume first. The routes live beside the directory
// they write to — the same <root>/uploads tree registerSPA serves read-only —
// and each one names the permission its folder belongs to: 封面图 is a product
// edit, a site Logo is a settings edit, so 运营 can upload covers and never
// repaints the storefront.
//
// A caller chooses nothing about where a file lands: the folder is fixed when
// the route is registered, the name is generated here, and the bytes only count
// as an image if their own header says so.
func registerUploads(router *gin.Engine, rootDir string, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	dir := filepath.Join(rootDir, "uploads")
	gate := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	admin := router.Group("/api/v1/admin/uploads", middleware.JWTMiddleware(jwtConfig))
	admin.POST("/products", gate("products", "manage"), receiveImage(upload.New(dir, "products")))
	admin.POST("/site", gate("settings", "manage"), receiveImage(upload.New(dir, "site")))
}

// multipartSlack is the boundary and header bytes a 2 MB picture arrives with.
// Anything past it is not one picture that happens to be large.
const multipartSlack = 64 << 10

// receiveImage reads the one file field an upload form carries and answers with
// the address to paste into an image field.
func receiveImage(store *upload.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > upload.MaxBytes+multipartSlack {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": upload.ErrTooLarge.Error()})
			return
		}

		reader, err := c.Request.MultipartReader()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": upload.ErrEmpty.Error()})
			return
		}
		var data []byte
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": upload.ErrEmpty.Error()})
				return
			}
			if part.FormName() != "image" {
				continue
			}
			// One byte over the limit is enough to know: ReadAll stops after
			// MaxBytes+1 so a 4 GB body cannot be streamed into memory to be
			// refused afterwards.
			data, err = io.ReadAll(io.LimitReader(part, upload.MaxBytes+1))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": upload.ErrEmpty.Error()})
				return
			}
			break
		}

		result, err := store.Save(data)
		switch {
		case errors.Is(err, upload.ErrTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		case err != nil:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, result)
		}
	}
}
