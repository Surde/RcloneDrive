package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// appVersion 是当前版本，检查更新时与 GitHub 最新 release 比较。
// 发布构建由 CI 按 tag 注入（-ldflags "-X main.appVersion=x.y.z"），
// 这里是没有注入时的默认版本。
var appVersion = "0.1.0"

const releasesAPI = "https://api.github.com/repos/younland/RcloneDrive/releases/latest"

// UpdateResult 描述一次检查更新的结果，通过 "update:result" 事件传给前端。
type UpdateResult struct {
	Status  string `json:"status"` // update | latest | none | error
	Current string `json:"current"`
	Latest  string `json:"latest"`
	URL     string `json:"url"`
	Message string `json:"message"`
}

// checkUpdate 查询 GitHub 上最新的 release 并与当前版本比较。
func checkUpdate() UpdateResult {
	result := UpdateResult{Status: "error", Current: appVersion}

	req, err := http.NewRequest(http.MethodGet, releasesAPI, nil)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RcloneDrive/"+appVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Message = "无法连接更新服务：" + err.Error()
		return result
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		result.Status = "none"
		result.Message = "项目暂未发布版本"
		return result
	default:
		result.Message = fmt.Sprintf("更新服务返回 %s", resp.Status)
		return result
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		result.Message = "解析更新信息失败：" + err.Error()
		return result
	}

	latest := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	result.Latest = latest
	result.URL = payload.HTMLURL
	if latest == "" {
		result.Status = "none"
		result.Message = "项目暂未发布版本"
		return result
	}
	if compareVersions(latest, appVersion) > 0 {
		result.Status = "update"
		result.Message = "发现新版本 v" + latest
		return result
	}
	result.Status = "latest"
	result.Message = "已是最新版本 v" + appVersion
	return result
}

// compareVersions 比较形如 1.2.3 的版本号，a 大于 b 时返回正数。
func compareVersions(a, b string) int {
	partA := strings.Split(a, ".")
	partB := strings.Split(b, ".")
	for i := 0; i < len(partA) || i < len(partB); i++ {
		var numA, numB int
		if i < len(partA) {
			numA, _ = strconv.Atoi(strings.TrimSpace(partA[i]))
		}
		if i < len(partB) {
			numB, _ = strconv.Atoi(strings.TrimSpace(partB[i]))
		}
		if numA != numB {
			return numA - numB
		}
	}
	return 0
}
