package api

import (
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Resinat/Resin/internal/service"
)

type nodeExportFormat string

const (
	nodeExportFormatSingboxJSON nodeExportFormat = "singbox_json"
	nodeExportFormatProxyURI    nodeExportFormat = "proxy_uri"
)

type exportNodeTLS struct {
	Enabled    bool   `json:"enabled"`
	ServerName string `json:"server_name"`
	Insecure   bool   `json:"insecure"`
}

type exportNodeRaw struct {
	Type       string         `json:"type"`
	Server     string         `json:"server"`
	ServerPort int            `json:"server_port"`
	Username   string         `json:"username"`
	Password   string         `json:"password"`
	TLS        *exportNodeTLS `json:"tls"`
}

func parseNodeExportFormat(raw string) (nodeExportFormat, bool) {
	format := nodeExportFormat(strings.ToLower(strings.TrimSpace(raw)))
	if format == "" {
		return nodeExportFormatSingboxJSON, true
	}
	switch format {
	case nodeExportFormatSingboxJSON, nodeExportFormatProxyURI:
		return format, true
	default:
		return "", false
	}
}

func formatNodeExportFilename(format nodeExportFormat) string {
	base := "resin-nodes-" + time.Now().UTC().Format("20060102-150405")
	if format == nodeExportFormatProxyURI {
		return base + ".txt"
	}
	return base + ".json"
}

func renderNodeExport(cp *service.ControlPlaneService, nodes []service.NodeSummary, format nodeExportFormat) ([]byte, string, string, error) {
	switch format {
	case nodeExportFormatProxyURI:
		lines := make([]string, 0, len(nodes))
		for _, nodeSummary := range nodes {
			raw, err := cp.GetNodeRawOptions(nodeSummary.NodeHash)
			if err != nil {
				return nil, "", "", err
			}
			line, ok, err := convertNodeRawToProxyURI(raw)
			if err != nil {
				return nil, "", "", err
			}
			if !ok {
				continue
			}
			lines = append(lines, line)
		}
		body := []byte(strings.Join(lines, "\n"))
		if len(lines) > 0 {
			body = append(body, '\n')
		}
		return body, "text/plain; charset=utf-8", formatNodeExportFilename(format), nil
	default:
		outbounds := make([]json.RawMessage, 0, len(nodes))
		for _, nodeSummary := range nodes {
			raw, err := cp.GetNodeRawOptions(nodeSummary.NodeHash)
			if err != nil {
				return nil, "", "", err
			}
			outbounds = append(outbounds, raw)
		}
		body, err := json.Marshal(nodeExportResponse{Outbounds: outbounds})
		if err != nil {
			return nil, "", "", invalidArgumentError("failed to encode node export")
		}
		return body, "application/json; charset=utf-8", formatNodeExportFilename(format), nil
	}
}

func convertNodeRawToProxyURI(raw json.RawMessage) (string, bool, error) {
	var outbound exportNodeRaw
	if err := json.Unmarshal(raw, &outbound); err != nil {
		return "", false, invalidArgumentError("node export: invalid raw node json")
	}

	nodeType := strings.ToLower(strings.TrimSpace(outbound.Type))
	server := strings.TrimSpace(outbound.Server)
	if nodeType == "" || server == "" || outbound.ServerPort <= 0 {
		return "", false, nil
	}

	scheme := ""
	switch nodeType {
	case "socks":
		scheme = "socks5"
	case "http":
		scheme = "http"
		if outbound.TLS != nil && outbound.TLS.Enabled {
			scheme = "https"
		}
	default:
		return "", false, nil
	}

	uri := &url.URL{
		Scheme: scheme,
		Host:   joinHostPort(server, outbound.ServerPort),
	}
	if outbound.Username != "" || outbound.Password != "" {
		if outbound.Password != "" {
			uri.User = url.UserPassword(outbound.Username, outbound.Password)
		} else {
			uri.User = url.User(outbound.Username)
		}
	}

	if scheme == "https" {
		query := url.Values{}
		if outbound.TLS != nil {
			serverName := strings.TrimSpace(outbound.TLS.ServerName)
			if serverName != "" && !strings.EqualFold(serverName, server) {
				query.Set("sni", serverName)
			}
			if outbound.TLS.Insecure {
				query.Set("allowInsecure", "1")
			}
		}
		uri.RawQuery = query.Encode()
	}

	return uri.String(), true, nil
}

func joinHostPort(host string, port int) string {
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}
