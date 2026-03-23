package utils

import (
	"fmt"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"github.com/sirupsen/logrus"
	"github.com/xrcuo/xrcuo-lib/config"
)

type RegionParts struct {
	Country  string
	Province string
	City     string
	Isp      string
}

var ip2regionService *service.Ip2Region

func InitIP2Region() error {
	v4DBPath := config.GetIP2RegionV4DBPath()
	v6DBPath := config.GetIP2RegionV6DBPath()

	logrus.Infof("Initializing IP2Region: IPv4=%s, IPv6=%s", v4DBPath, v6DBPath)

	v4Config, err := service.NewV4Config(service.VIndexCache, v4DBPath, 20)
	if err != nil {
		return fmt.Errorf("failed to create IPv4 config: %v", err)
	}

	v6Config, err := service.NewV6Config(service.VIndexCache, v6DBPath, 20)
	if err != nil {
		logrus.Warnf("Failed to create IPv6 config, using IPv4 only: %v", err)
		ip2regionService, err = service.NewIp2Region(v4Config, nil)
	} else {
		ip2regionService, err = service.NewIp2Region(v4Config, v6Config)
	}

	if err != nil {
		return fmt.Errorf("failed to create IP2Region service: %v", err)
	}

	logrus.Info("IP2Region initialized successfully")
	return nil
}

func GetRegionByIP(ip string) (RegionParts, error) {
	if IsPrivateIP(ip) {
		return RegionParts{
			Country:  "内网IP",
			Province: "",
			City:     "",
			Isp:      "",
		}, nil
	}

	regionRaw, err := ip2regionService.SearchByStr(ip)
	if err != nil {
		return RegionParts{}, fmt.Errorf("IP search failed: %v", err)
	}

	return parseRegionRaw(regionRaw), nil
}

func CloseIP2Region() {
	if ip2regionService != nil {
		ip2regionService.Close()
		logrus.Info("IP2Region closed")
	}
}

func parseRegionRaw(regionRaw string) RegionParts {
	parts := strings.Split(regionRaw, "|")
	result := RegionParts{}

	switch len(parts) {
	case 4:
		result.Isp = parseEmptyField(parts[3])
		result.City = parseEmptyField(parts[2])
		result.Province = parseEmptyField(parts[1])
		result.Country = parseEmptyField(parts[0])
	case 3:
		result.City = parseEmptyField(parts[2])
		result.Province = parseEmptyField(parts[1])
		result.Country = parseEmptyField(parts[0])
	case 2:
		result.Province = parseEmptyField(parts[1])
		result.Country = parseEmptyField(parts[0])
	case 1:
		result.Country = parseEmptyField(parts[0])
	}

	return result
}

func parseEmptyField(field string) string {
	if field == "0" || field == "" || field == "未知" {
		return ""
	}
	return field
}
