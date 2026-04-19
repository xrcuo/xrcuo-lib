package common

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"github.com/sirupsen/logrus"
)

//go:embed data/ip2region_v4.xdb
//go:embed data/ip2region_v6.xdb
var embeddedDB embed.FS

// RegionParts 地区结构化数据
type RegionParts struct {
	Country  string // 国家
	Province string // 省份
	City     string // 城市
	Isp      string // 运营商
}

// 全局ip2region服务
var ip2regionService *service.Ip2Region
var tempDir string

// InitIP2Region 初始化IP2Region服务
func InitIP2Region() error {
	logrus.Info("开始初始化IP2Region服务（使用内嵌数据库）")

	// 创建临时目录
	var err error
	tempDir, err = os.MkdirTemp("", "ip2region-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	logrus.Infof("创建临时目录: %s", tempDir)

	// 提取 IPv4 数据库文件
	v4DBPath := filepath.Join(tempDir, "ip2region_v4.xdb")
	if err := extractEmbeddedFile("data/ip2region_v4.xdb", v4DBPath); err != nil {
		return fmt.Errorf("提取IPv4数据库失败: %v", err)
	}

	// 提取 IPv6 数据库文件
	v6DBPath := filepath.Join(tempDir, "ip2region_v6.xdb")
	if err := extractEmbeddedFile("data/ip2region_v6.xdb", v6DBPath); err != nil {
		logrus.Warnf("提取IPv6数据库失败，将仅使用IPv4: %v", err)
	}

	// 创建v4配置：指定缓存策略和v4的xdb文件路径
	v4Config, err := service.NewV4Config(service.VIndexCache, v4DBPath, 20)
	if err != nil {
		return fmt.Errorf("创建IPv4配置失败: %v", err)
	}

	// 尝试创建v6配置，如果失败则只使用v4配置
	v6Config, err := service.NewV6Config(service.VIndexCache, v6DBPath, 20)
	if err != nil {
		logrus.Warnf("创建IPv6配置失败，将只使用IPv4配置: %v", err)
		// 通过配置创建Ip2Region查询服务（只使用v4配置）
		ip2regionService, err = service.NewIp2Region(v4Config, nil)
	} else {
		// 通过配置创建Ip2Region查询服务（同时使用v4和v6配置）
		ip2regionService, err = service.NewIp2Region(v4Config, v6Config)
	}

	if err != nil {
		return fmt.Errorf("创建IP2Region服务失败: %v", err)
	}

	logrus.Info("IP2Region服务初始化成功")
	return nil
}

// GetRegionByIP 根据IP获取地区信息（支持内网IP识别）
func GetRegionByIP(ip string) (RegionParts, error) {
	// 内网IP直接返回固定结果
	if IsPrivateIP(ip) {
		return RegionParts{
			Country:  "内网IP",
			Province: "",
			City:     "",
			Isp:      "",
		}, nil
	}

	// 执行查询
	regionRaw, err := ip2regionService.Search(ip)
	if err != nil {
		return RegionParts{}, fmt.Errorf("IP查询失败：%v", err)
	}

	logrus.Debugf("IP查询原始结果: IP=%s, Raw=%s", ip, regionRaw)

	// 解析地区字符串（适配4段格式：国家|省份|城市|ISP）
	result := parseRegionRaw(regionRaw)
	logrus.Debugf("IP查询解析结果: %+v", result)

	return result, nil
}

// CloseIP2Region 关闭IP2Region服务并清理临时文件
func CloseIP2Region() {
	if ip2regionService != nil {
		ip2regionService.Close()
		logrus.Info("IP2Region服务已关闭")
	}

	// 清理临时目录
	if tempDir != "" {
		if err := os.RemoveAll(tempDir); err != nil {
			logrus.Warnf("清理临时目录失败: %v", err)
		} else {
			logrus.Infof("清理临时目录成功: %s", tempDir)
		}
	}
}

// parseRegionRaw 解析ip2region原始返回值
func parseRegionRaw(regionRaw string) RegionParts {
	parts := strings.Split(regionRaw, "|")
	result := RegionParts{}

	// 定义一个辅助函数，保留更多数据并过滤掉无效值
	keepField := func(field string) string {
		field = strings.TrimSpace(field)
		if field == "0" || field == "" || strings.EqualFold(field, "reserved") {
			return ""
		}
		return field
	}

	switch len(parts) {
	case 5:
		result.Isp = keepField(parts[3])
		result.City = keepField(parts[2])
		result.Province = keepField(parts[1])
		result.Country = keepField(parts[0])
	case 4:
		result.Isp = keepField(parts[3])
		result.City = keepField(parts[2])
		result.Province = keepField(parts[1])
		result.Country = keepField(parts[0])
	case 3:
		result.City = keepField(parts[2])
		result.Province = keepField(parts[1])
		result.Country = keepField(parts[0])
	case 2:
		result.Province = keepField(parts[1])
		result.Country = keepField(parts[0])
	case 1:
		result.Country = keepField(parts[0])
	}

	return result
}

// JoinNonEmpty 合并非空字符串（忽略空值）
func JoinNonEmpty(strs []string, sep string) string {
	var result []string
	for _, s := range strs {
		if s != "" {
			result = append(result, s)
		}
	}
	return strings.Join(result, sep)
}

// extractEmbeddedFile 从 embed.FS 中提取文件到指定路径
func extractEmbeddedFile(embedPath, destPath string) error {
	logrus.Infof("正在提取内嵌文件: %s -> %s", embedPath, destPath)

	data, err := embeddedDB.ReadFile(embedPath)
	if err != nil {
		return fmt.Errorf("读取内嵌文件失败: %v", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	logrus.Infof("文件提取成功: %s", destPath)
	return nil
}
