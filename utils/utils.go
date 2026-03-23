package utils

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func IsPrivateIP(ip string) bool {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}
	return ipAddr.IsPrivate()
}

func ResolveTarget(target string) (string, error) {
	if net.ParseIP(target) != nil {
		return target, nil
	}

	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", target)
	if err != nil {
		return "", fmt.Errorf("DNS resolution failed: %v", err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IPv4 address resolved")
	}

	return ips[0].String(), nil
}

func StrToInt(str string, defaultValue int) int {
	if str == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return defaultValue
	}
	return val
}

func FormatDelay(delay int64) string {
	if delay == 0 {
		return "timeout"
	}
	ms := float64(delay) / 1000.0
	return fmt.Sprintf("%.2fms", ms)
}

func JoinNonEmpty(strs []string, sep string) string {
	var result []string
	for _, s := range strs {
		if s != "" {
			result = append(result, s)
		}
	}
	return strings.Join(result, sep)
}
