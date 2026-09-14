package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"BackendTemplate/pkg/c2profile"
)

// ListC2Profiles 已加载 profile 列表
func ListC2Profiles(c *gin.Context) {
	names := c2profile.List()
	type item struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var out []item
	for _, n := range names {
		p := c2profile.Get(n)
		it := item{Name: n}
		if p != nil {
			it.Description = p.Description
		}
		out = append(out, it)
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": out})
}

// PreviewC2Profile 返回 profile YAML 原文（编辑/预览）
func PreviewC2Profile(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		name = "default"
	}
	raw, err := c2profile.Raw(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "message": "profile 不存在: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": raw})
}

// SaveC2Profile 保存 profile YAML（写 profiles/<name>.yaml，3 秒内热加载）
func SaveC2Profile(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少 name/content"})
		return
	}
	if err := c2profile.Save(req.Name, req.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "保存失败: " + err.Error()})
		return
	}
	_ = c2profile.Reload()
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"name": req.Name}})
}
