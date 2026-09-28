package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/platform/upload"
)

// registerUploads gives the shop a way to put a picture on the shelf without
// shell-ing a file into the volume first. The routes live beside the directory
// they write to — the same <root>/uploads tree registerSPA serves read-only —
// and each one names the permission its folder belongs to: 封面图 is a product
// edit, a site Logo is a settings edit, so 运营 can upload covers and never
// repaints the storefront.
//
// The third route is not a back office one: a buyer's avatar belongs to the
// buyer, so it sits under /auth/me behind a session and nothing else.
//
// A caller chooses nothing about where a file lands: the folder is fixed when the
// route is registered, the name is generated here, and the bytes only count as an
// image if their own header says so.
func registerUploads(router *gin.Engine, rootDir string, jwtConfig *config.JWTConfig, accounts middleware.AccountReader, profiles avatarAccounts) {
	dir := filepath.Join(rootDir, "uploads")
	gate := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	admin := router.Group("/api/v1/admin/uploads", middleware.JWTMiddleware(jwtConfig))
	admin.POST("/products", gate("products", "manage"), receiveImage(upload.New(dir, "products")))
	admin.POST("/site", gate("settings", "manage"), receiveImage(upload.New(dir, "site")))

	own := router.Group("/api/v1/auth/me", middleware.JWTMiddleware(jwtConfig))
	own.POST("/avatar", receiveAvatar(upload.New(dir, "avatars"), profiles))
}

// avatarAccounts is the part of the identity module an avatar upload needs: the
// account whose picture is being replaced, and the profile write that points it
// at the new one.
type avatarAccounts interface {
	Me(ctx context.Context, userID uint) (*domain.User, error)
	UpdateProfile(ctx context.Context, userID uint, edit domain.ProfileEdit) (*domain.User, error)
}

// multipartSlack is the boundary and header bytes a 2 MB picture arrives with.
// Anything past it is not one picture that happens to be large.
const multipartSlack = 64 << 10

// receiveImage answers with the address to paste into an image field.
func receiveImage(store *upload.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := readImage(c)
		if err != nil {
			refuse(c, err)
			return
		}
		result, err := store.Save(data)
		if err != nil {
			refuse(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// receiveAvatar keeps the picture and the account in step: the address is written
// straight into the profile, because an avatar is not a value one copies out of a
// response and into a form. The account is pointed at the new file first, so the
// old one is only ever deleted once nothing refers to it any more.
func receiveAvatar(store *upload.Store, profiles avatarAccounts) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := readImage(c)
		if err != nil {
			refuse(c, err)
			return
		}
		result, err := store.Save(data)
		if err != nil {
			refuse(c, err)
			return
		}

		userID, ok := userIDOf(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录，再上传头像。", "code": "unauthenticated"})
			return
		}
		current, err := profiles.Me(c.Request.Context(), userID)
		if err != nil {
			log.Printf("[upload] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号资料失败，请稍后再试。", "code": "internal_error"})
			return
		}
		user, err := profiles.UpdateProfile(c.Request.Context(), userID, domain.ProfileEdit{AvatarURL: &result.URL})
		if err != nil {
			// The new file is on disk but unreferenced; a refusal here would leave
			// the shop with a picture nobody points at, so the bytes are dropped too.
			store.Remove(result.URL)
			refuse(c, err)
			return
		}
		store.Remove(current.AvatarURL)
		c.JSON(http.StatusOK, gin.H{"user": user, "url": result.URL, "width": result.Width, "height": result.Height})
	}
}

// userIDOf reads the account the session middleware put on the request. The
// avatar route has no permission to check, so this is the whole of its gate: the
// path can only ever touch the signed-in account's own picture.
func userIDOf(c *gin.Context) (uint, bool) {
	value, exists := c.Get(middleware.UserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok && userID != 0
}

// readImage pulls the one file field an upload form carries and answers with the
// bytes, or with the reason there are none worth sending.
func readImage(c *gin.Context) ([]byte, error) {
	if c.Request.ContentLength > upload.MaxBytes+multipartSlack {
		return nil, upload.ErrTooLarge
	}

	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, upload.ErrEmpty
	}
	for {
		part, err := reader.NextPart()
		if err != nil {
			// io.EOF means the form carried no image field at all.
			return nil, upload.ErrEmpty
		}
		if part.FormName() != "image" {
			continue
		}
		// One byte over the limit is enough to know: ReadAll stops after
		// MaxBytes+1 so a 4 GB body cannot be streamed into memory to be
		// refused afterwards.
		data, err := io.ReadAll(io.LimitReader(part, upload.MaxBytes+1))
		if err != nil {
			return nil, upload.ErrEmpty
		}
		if len(data) == 0 {
			return nil, upload.ErrEmpty
		}
		return data, nil
	}
}

// refuse is the one answer every upload route gives. The reasons the platform
// package names are already the shop's own words, so this only adds the status
// and the machine code, keeps a rejected profile edit to the part after the
// sentinel, and turns an unrecognised fault into a plain 500 whose cause stays in
// the log instead of on the page.
func refuse(c *gin.Context, err error) {
	status, code := uploadRefusal(err)
	message := err.Error()
	if _, tail, found := strings.Cut(message, ": "); code == "invalid_input" && found && strings.TrimSpace(tail) != "" {
		message = tail
	}
	if code == "internal_error" {
		log.Printf("[upload] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		message = "这张图片没能存进商店的文件里，请稍后再试。"
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

// uploadRefusal maps each refusal to the status the page retries on: an
// over-sized file is the buyer's or the shop's own to shrink (413), a file that
// is not a picture is a bad submission (400), anything else is the shop's fault
// and says so as a 500.
func uploadRefusal(err error) (int, string) {
	switch {
	case errors.Is(err, upload.ErrEmpty):
		return http.StatusBadRequest, "empty_upload"
	case errors.Is(err, upload.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "image_too_large"
	case errors.Is(err, upload.ErrNotImage), errors.Is(err, upload.ErrHuge):
		return http.StatusBadRequest, "invalid_image"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input"
	}
	return http.StatusInternalServerError, "internal_error"
}
