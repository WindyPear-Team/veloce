package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/config"
	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const pluginMaxPackageBytes int64 = 100 << 20

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,79}$`)

type pluginAPI struct{}

type PluginManifest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	WASM        string          `json:"wasm"`
	Permissions []string        `json:"permissions"`
	Hooks       []PluginHook    `json:"hooks"`
	Frontend    json.RawMessage `json:"frontend"`
	Settings    json.RawMessage `json:"settings"`
}

type PluginHook struct {
	Point string `json:"point"`
	Mode  string `json:"mode"`
}

type pluginListItem struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Enabled     bool            `json:"enabled"`
	Permissions []string        `json:"permissions"`
	Hooks       []PluginHook    `json:"hooks"`
	Frontend    json.RawMessage `json:"frontend,omitempty"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func init() {
	RegisterUserRouteHook(registerPluginUserRoutes)
}

func registerPluginUserRoutes(group *gin.RouterGroup) {
	api := &pluginAPI{}
	plugins := group.Group("/plugins")
	plugins.GET("", api.listPlugins)
	plugins.POST("", api.installPlugin)
	plugins.GET("/frontend", api.frontendExtensions)
	plugins.POST("/:id/enable", api.enablePlugin)
	plugins.POST("/:id/disable", api.disablePlugin)
	plugins.DELETE("/:id", api.uninstallPlugin)
	plugins.GET("/:id/settings", api.getPluginSettings)
	plugins.PUT("/:id/settings", api.updatePluginSettings)
	plugins.POST("/:id/actions/:action", api.runPluginAction)
}

func (api *pluginAPI) listPlugins(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var plugins []model.Plugin
	if err := model.DB.Order("created_at desc").Find(&plugins).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list plugins"})
		return
	}
	states := userPluginStates(user.ID)
	items := make([]pluginListItem, 0, len(plugins))
	for _, plugin := range plugins {
		items = append(items, pluginListResponse(plugin, pluginUserEnabled(plugin, states)))
	}
	c.JSON(http.StatusOK, gin.H{"plugins": items})
}

func (api *pluginAPI) frontendExtensions(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var plugins []model.Plugin
	if err := model.DB.Where("enabled = ?", true).Order("created_at asc").Find(&plugins).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list plugins"})
		return
	}
	states := userPluginStates(user.ID)
	items := make([]pluginListItem, 0, len(plugins))
	for _, plugin := range plugins {
		if !pluginUserEnabled(plugin, states) || strings.TrimSpace(plugin.FrontendJSON) == "" {
			continue
		}
		items = append(items, pluginListResponse(plugin, true))
	}
	c.JSON(http.StatusOK, gin.H{"plugins": items})
}

func (api *pluginAPI) installPlugin(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plugin package file is required"})
		return
	}
	if file.Size > pluginMaxPackageBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plugin package is too large"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open plugin package"})
		return
	}
	defer src.Close()

	tmpRoot := filepath.Join(config.DataPath, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare plugin temp directory"})
		return
	}
	tmpDir, err := os.MkdirTemp(tmpRoot, "plugin-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare plugin temp directory"})
		return
	}
	defer os.RemoveAll(tmpDir)

	manifest, manifestRaw, wasmRel, err := prepareUploadedPlugin(c.Request.Context(), src, file.Filename, file.Size, tmpDir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validatePluginManifest(manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wasmPath := ""
	if wasmRel != "" {
		resolved, err := safePluginFilePath(tmpDir, wasmRel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid WASM path"})
			return
		}
		if stat, err := os.Stat(resolved); err == nil && !stat.IsDir() {
			wasmPath = filepath.Join(pluginInstallDir(manifest.ID), filepath.FromSlash(path.Clean(strings.ReplaceAll(wasmRel, "\\", "/"))))
		}
	}

	finalDir := pluginInstallDir(manifest.ID)
	if err := os.RemoveAll(finalDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to replace old plugin files"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare plugin directory"})
		return
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to install plugin files"})
		return
	}

	permissionsJSON := mustJSON(manifest.Permissions)
	hooksJSON := mustJSON(manifest.Hooks)
	plugin := model.Plugin{
		ID:              manifest.ID,
		Name:            manifest.Name,
		Version:         manifest.Version,
		Description:     manifest.Description,
		Author:          manifest.Author,
		Enabled:         true,
		ManifestJSON:    string(manifestRaw),
		PermissionsJSON: permissionsJSON,
		HooksJSON:       hooksJSON,
		FrontendJSON:    string(manifest.Frontend),
		SettingsJSON:    string(manifest.Settings),
		Path:            finalDir,
		WASMPath:        wasmPath,
	}
	if err := model.DB.Where(&model.Plugin{ID: plugin.ID}).Assign(plugin).FirstOrCreate(&plugin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save plugin"})
		return
	}
	if err := setUserPluginEnabled(user.ID, plugin.ID, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable plugin"})
		return
	}
	if err := InitializePluginWASM(c.Request.Context(), plugin); err != nil {
		model.DB.Model(&plugin).Update("last_error", err.Error())
		recordPluginLog(user.ID, plugin.ID, "warn", "wasm_init_failed", err.Error(), "")
	} else if plugin.WASMPath != "" {
		model.DB.Model(&plugin).Update("last_error", "")
		recordPluginLog(user.ID, plugin.ID, "info", "wasm_init", "WASM plugin initialized", "")
	}
	c.JSON(http.StatusOK, gin.H{"plugin": pluginListResponse(plugin, true)})
}

func prepareUploadedPlugin(ctx context.Context, src io.Reader, filename string, size int64, tmpDir string) (PluginManifest, []byte, string, error) {
	lower := strings.ToLower(strings.TrimSpace(filename))
	if strings.HasSuffix(lower, ".wasm") {
		wasmRel := "plugin.wasm"
		wasmPath := filepath.Join(tmpDir, wasmRel)
		if err := writeUploadedWASM(wasmPath, src); err != nil {
			return PluginManifest{}, nil, "", err
		}
		manifest, raw, err := ReadPluginManifestFromWASM(ctx, wasmPath)
		if err != nil {
			return PluginManifest{}, raw, "", err
		}
		manifest = normalizePluginManifest(manifest)
		manifest.WASM = wasmRel
		return manifest, raw, wasmRel, nil
	}

	if err := extractPluginPackage(src, filename, size, tmpDir); err != nil {
		return PluginManifest{}, nil, "", err
	}
	manifest, raw, err := readPluginManifest(tmpDir)
	if err == nil {
		manifest = normalizePluginManifest(manifest)
		wasmRel := strings.TrimSpace(manifest.WASM)
		if wasmRel == "" {
			wasmRel = "plugin.wasm"
		}
		manifest.WASM = wasmRel
		return manifest, raw, wasmRel, nil
	}
	wasmRel, err := discoverSingleWASM(tmpDir)
	if err != nil {
		return PluginManifest{}, nil, "", errors.New("plugin package must contain plugin.json or a single WASM with plugin_manifest export")
	}
	wasmPath, err := safePluginFilePath(tmpDir, wasmRel)
	if err != nil {
		return PluginManifest{}, nil, "", err
	}
	manifest, raw, err = ReadPluginManifestFromWASM(ctx, wasmPath)
	if err != nil {
		return PluginManifest{}, raw, "", err
	}
	manifest = normalizePluginManifest(manifest)
	manifest.WASM = wasmRel
	return manifest, raw, wasmRel, nil
}

func (api *pluginAPI) enablePlugin(c *gin.Context) {
	api.setPluginEnabled(c, true)
}

func (api *pluginAPI) disablePlugin(c *gin.Context) {
	api.setPluginEnabled(c, false)
}

func (api *pluginAPI) setPluginEnabled(c *gin.Context, enabled bool) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	plugin, ok := loadPlugin(c)
	if !ok {
		return
	}
	if err := setUserPluginEnabled(user.ID, plugin.ID, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update plugin state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugin": pluginListResponse(plugin, enabled)})
}

func (api *pluginAPI) uninstallPlugin(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	plugin, ok := loadPlugin(c)
	if !ok {
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plugin_id = ?", plugin.ID).Delete(&model.UserPluginState{}).Error; err != nil {
			return err
		}
		if err := tx.Where("plugin_id = ?", plugin.ID).Delete(&model.UserPluginConfig{}).Error; err != nil {
			return err
		}
		if err := tx.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginKV{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&plugin).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to uninstall plugin"})
		return
	}
	_ = os.RemoveAll(plugin.Path)
	_ = os.RemoveAll(filepath.Join(config.DataPath, "plugin-data", fmt.Sprint(user.ID), plugin.ID))
	recordPluginLog(user.ID, plugin.ID, "info", "uninstall", "Plugin uninstalled", "")
	c.JSON(http.StatusOK, gin.H{"message": "Plugin uninstalled"})
}

func (api *pluginAPI) getPluginSettings(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	plugin, ok := loadPlugin(c)
	if !ok {
		return
	}
	var cfg model.UserPluginConfig
	_ = model.DB.Where("user_id = ? AND plugin_id = ?", user.ID, plugin.ID).Limit(1).Find(&cfg).Error
	c.JSON(http.StatusOK, gin.H{
		"schema": json.RawMessage(nonEmptyJSON(plugin.SettingsJSON, "{}")),
		"config": json.RawMessage(nonEmptyJSON(cfg.ConfigJSON, "{}")),
	})
}

func (api *pluginAPI) updatePluginSettings(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	plugin, ok := loadPlugin(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read settings"})
		return
	}
	var payload map[string]interface{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Settings must be a JSON object"})
			return
		}
	}
	raw := mustJSON(payload)
	cfg := model.UserPluginConfig{UserID: user.ID, PluginID: plugin.ID}
	if err := model.DB.Where(&model.UserPluginConfig{UserID: user.ID, PluginID: plugin.ID}).
		Assign(model.UserPluginConfig{ConfigJSON: raw}).
		FirstOrCreate(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": json.RawMessage(nonEmptyJSON(cfg.ConfigJSON, "{}"))})
}

func (api *pluginAPI) runPluginAction(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	plugin, ok := loadPlugin(c)
	if !ok {
		return
	}
	states := userPluginStates(user.ID)
	if !pluginUserEnabled(plugin, states) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Plugin is disabled"})
		return
	}
	var payload map[string]interface{}
	_ = c.ShouldBindJSON(&payload)
	result, err := InvokePluginAction(c.Request.Context(), plugin, user.ID, c.Param("action"), payload)
	if err != nil {
		recordPluginLog(user.ID, plugin.ID, "error", "action_failed", err.Error(), mustJSON(gin.H{"action": c.Param("action")}))
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func loadPlugin(c *gin.Context) (model.Plugin, bool) {
	id := strings.TrimSpace(c.Param("id"))
	var plugin model.Plugin
	if id == "" || model.DB.Where("id = ?", id).Limit(1).Find(&plugin).Error != nil || plugin.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plugin not found"})
		return model.Plugin{}, false
	}
	return plugin, true
}

func pluginListResponse(plugin model.Plugin, enabled bool) pluginListItem {
	return pluginListItem{
		ID:          plugin.ID,
		Name:        plugin.Name,
		Version:     plugin.Version,
		Description: plugin.Description,
		Author:      plugin.Author,
		Enabled:     enabled,
		Permissions: decodePluginStringList(plugin.PermissionsJSON),
		Hooks:       decodeHooks(plugin.HooksJSON),
		Frontend:    json.RawMessage(nonEmptyJSON(plugin.FrontendJSON, "null")),
		Settings:    json.RawMessage(nonEmptyJSON(plugin.SettingsJSON, "null")),
		LastError:   plugin.LastError,
		CreatedAt:   plugin.CreatedAt,
		UpdatedAt:   plugin.UpdatedAt,
	}
}

func userPluginStates(userID uint) map[string]bool {
	var states []model.UserPluginState
	_ = model.DB.Where("user_id = ?", userID).Find(&states).Error
	result := map[string]bool{}
	for _, state := range states {
		result[state.PluginID] = state.Enabled
	}
	return result
}

func pluginUserEnabled(plugin model.Plugin, states map[string]bool) bool {
	if !plugin.Enabled {
		return false
	}
	enabled, ok := states[plugin.ID]
	return ok && enabled
}

func setUserPluginEnabled(userID uint, pluginID string, enabled bool) error {
	state := model.UserPluginState{UserID: userID, PluginID: pluginID}
	return model.DB.Where(&model.UserPluginState{UserID: userID, PluginID: pluginID}).
		Assign(model.UserPluginState{Enabled: enabled}).
		FirstOrCreate(&state).Error
}

func readPluginManifest(root string) (PluginManifest, []byte, error) {
	manifestPath := filepath.Join(root, "plugin.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return PluginManifest{}, nil, errors.New("plugin.json is required")
	}
	var manifest PluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return PluginManifest{}, nil, fmt.Errorf("invalid plugin.json: %w", err)
	}
	return manifest, raw, nil
}

func validatePluginManifest(manifest PluginManifest) error {
	if !pluginIDPattern.MatchString(manifest.ID) {
		return errors.New("plugin id must match ^[A-Za-z0-9][A-Za-z0-9_-]{1,79}$")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("plugin name is required")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return errors.New("plugin version is required")
	}
	for _, permission := range manifest.Permissions {
		if strings.TrimSpace(permission) == "" {
			return errors.New("plugin permissions cannot contain empty values")
		}
	}
	for _, hook := range manifest.Hooks {
		if strings.TrimSpace(hook.Point) == "" {
			return errors.New("plugin hook point is required")
		}
	}
	return nil
}

func normalizePluginManifest(manifest PluginManifest) PluginManifest {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Author = strings.TrimSpace(manifest.Author)
	manifest.WASM = strings.TrimSpace(manifest.WASM)
	for i := range manifest.Permissions {
		manifest.Permissions[i] = strings.TrimSpace(manifest.Permissions[i])
	}
	for i := range manifest.Hooks {
		manifest.Hooks[i].Point = strings.TrimSpace(manifest.Hooks[i].Point)
		manifest.Hooks[i].Mode = strings.TrimSpace(manifest.Hooks[i].Mode)
	}
	return manifest
}

func extractPluginPackage(src io.Reader, filename string, size int64, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(filename))
	limited := io.LimitReader(src, pluginMaxPackageBytes+1)
	switch {
	case strings.HasSuffix(name, ".wasm"):
		return writeUploadedWASM(filepath.Join(dest, "plugin.wasm"), limited)
	case strings.HasSuffix(name, ".zip"):
		tmp, err := os.CreateTemp("", "plugin-*.zip")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		written, copyErr := io.Copy(tmp, limited)
		closeErr := tmp.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > pluginMaxPackageBytes {
			return errors.New("plugin package is too large")
		}
		reader, err := zip.OpenReader(tmpName)
		if err != nil {
			return fmt.Errorf("invalid zip package: %w", err)
		}
		defer reader.Close()
		return extractZip(reader, dest)
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		gz, err := gzip.NewReader(limited)
		if err != nil {
			return fmt.Errorf("invalid tar.gz package: %w", err)
		}
		defer gz.Close()
		return extractTar(tar.NewReader(gz), dest)
	default:
		return errors.New("plugin package must be .wasm, .zip, .tar.gz, or .tgz")
	}
}

func writeUploadedWASM(target string, src io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(src, pluginMaxPackageBytes+1))
	if err != nil {
		return err
	}
	if written > pluginMaxPackageBytes {
		return errors.New("plugin WASM is too large")
	}
	return nil
}

func discoverSingleWASM(root string) (string, error) {
	var found []string
	if err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".wasm") {
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return "", err
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected exactly one WASM file, found %d", len(found))
	}
	return found[0], nil
}

func extractZip(reader *zip.ReadCloser, dest string) error {
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in plugin packages: %s", file.Name)
		}
		target, err := safePluginFilePath(dest, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(target, src, file.FileInfo().Mode().Perm())
		_ = src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(reader *tar.Reader, dest string) error {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir, tar.TypeReg, tar.TypeRegA:
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
		target, err := safePluginFilePath(dest, header.Name)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeArchiveFile(target, reader, os.FileMode(header.Mode).Perm()); err != nil {
			return err
		}
	}
}

func writeArchiveFile(target string, src io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(src, pluginMaxPackageBytes+1)); err != nil {
		return err
	}
	return nil
}

func safePluginFilePath(root, raw string) (string, error) {
	name := strings.ReplaceAll(raw, "\\", "/")
	clean := path.Clean(name)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") {
		return "", fmt.Errorf("unsafe plugin package path: %s", raw)
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, target)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe plugin package path: %s", raw)
	}
	return target, nil
}

func pluginInstallDir(id string) string {
	return filepath.Join(config.DataPath, "plugins", id)
}

func recordPluginLog(userID uint, pluginID, level, event, message, metadata string) {
	var uid *uint
	if userID != 0 {
		uid = &userID
	}
	_ = model.DB.Create(&model.PluginLog{
		UserID:   uid,
		PluginID: pluginID,
		Level:    level,
		Event:    event,
		Message:  message,
		Metadata: metadata,
	}).Error
}

func decodePluginStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	sort.Strings(values)
	return values
}

func decodeHooks(raw string) []PluginHook {
	var hooks []PluginHook
	_ = json.Unmarshal([]byte(raw), &hooks)
	return hooks
}

func mustJSON(value interface{}) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func nonEmptyJSON(raw, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	return raw
}

func pluginPackageChecksum(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
