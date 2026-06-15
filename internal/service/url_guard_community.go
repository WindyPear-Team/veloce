package service

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var ErrUnsafeURL = errors.New("target URL is blocked by SSRF protection")

type URLGuardOptions struct {
	AllowPrivateNetworks bool
	AllowedHosts         []string
	Resolve              bool
}

type URLGuardHooks struct {
	ValidateConfiguredHTTPURL    func(raw string) error
	ValidateConfiguredTCPAddress func(raw string) error
	ValidateConfiguredStatus     func(target string, checkType string) error
	ValidateOutboundHTTPURL      func(raw string, options URLGuardOptions) error
	CurrentOptions               func() URLGuardOptions
	Enabled                      func() bool
}

var urlGuardHooks URLGuardHooks

func RegisterURLGuardHooks(hooks URLGuardHooks) {
	urlGuardHooks = hooks
}

func ValidateConfiguredHTTPURL(raw string) error {
	if urlGuardHooks.ValidateConfiguredHTTPURL != nil {
		return urlGuardHooks.ValidateConfiguredHTTPURL(raw)
	}
	return validateHTTPURLSyntax(raw)
}

func ValidateConfiguredTCPAddress(raw string) error {
	if urlGuardHooks.ValidateConfiguredTCPAddress != nil {
		return urlGuardHooks.ValidateConfiguredTCPAddress(raw)
	}
	_, _, err := net.SplitHostPort(strings.TrimSpace(raw))
	return err
}

func ValidateConfiguredStatusTarget(target string, checkType string) error {
	if urlGuardHooks.ValidateConfiguredStatus != nil {
		return urlGuardHooks.ValidateConfiguredStatus(target, checkType)
	}
	if strings.EqualFold(strings.TrimSpace(checkType), StatusCheckTCP) {
		address, err := statusTCPGuardAddress(target)
		if err != nil {
			return err
		}
		return ValidateConfiguredTCPAddress(address)
	}
	return ValidateConfiguredHTTPURL(target)
}

func statusTCPGuardAddress(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("tcp target is required")
	}
	defaultPort := ""
	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
		target = parsed.Host
		switch parsed.Scheme {
		case "http":
			defaultPort = "80"
		case "https":
			defaultPort = "443"
		}
	}
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}
	if defaultPort == "" {
		return "", errors.New("tcp target must include a port")
	}
	return net.JoinHostPort(target, defaultPort), nil
}

func ValidateOutboundHTTPURL(raw string, options URLGuardOptions) error {
	if urlGuardHooks.ValidateOutboundHTTPURL != nil {
		return urlGuardHooks.ValidateOutboundHTTPURL(raw, options)
	}
	return validateHTTPURLSyntax(raw)
}

func CurrentURLGuardOptions() URLGuardOptions {
	if urlGuardHooks.CurrentOptions != nil {
		return urlGuardHooks.CurrentOptions()
	}
	return URLGuardOptions{}
}

func SSRFProtectionEnabled() bool {
	if urlGuardHooks.Enabled != nil {
		return urlGuardHooks.Enabled()
	}
	return false
}

func validateHTTPURLSyntax(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL must use http or https")
	}
	return nil
}
