package api

import (
	"encoding/json"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/Pukka922/edelweiss/sdk"
)

var (
	labelRe   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	numericRe = regexp.MustCompile(`^[0-9]+$`)
)

// maxHostnameLen is Linux HOST_NAME_MAX: sethostname(2) fails with EINVAL for longer names.
const maxHostnameLen = 64

// ValidHostname: RFC 1123 labels (lower case), at most 64 characters (the kernel limit),
// and a last label that is not all digits (RFC 1123 section 2.1: no dotted-decimal names).
func ValidHostname(h string) bool {
	if h == "" || len(h) > maxHostnameLen {
		return false
	}
	labels := strings.Split(h, ".")
	for _, label := range labels {
		if !labelRe.MatchString(label) {
			return false
		}
	}
	return !numericRe.MatchString(labels[len(labels)-1])
}

func (s *Service) setHostname(c fiber.Ctx) error {
	var req struct {
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return sdk.NewError(fiber.StatusBadRequest, "invalid_json", "request body must be valid JSON")
	}
	name := strings.ToLower(strings.TrimSpace(req.Hostname))
	if !ValidHostname(name) {
		return sdk.NewError(fiber.StatusBadRequest, "invalid_hostname", "use letters, digits and hyphens, labels separated by dots")
	}
	s.mu.Lock() // the previous name, the kernel change and the file must stay one unit
	defer s.mu.Unlock()
	previous, err := s.Hostname()
	if err != nil {
		return failed("hostname", err)
	}
	// the kernel first: a rejected name (EINVAL, EPERM) must not reach /etc/hostname
	if err := s.SetHostname(name); err != nil {
		slog.Error("setting hostname failed", "err", err)
		return sdk.NewError(fiber.StatusInternalServerError, "hostname_set_failed", "could not set the hostname: "+err.Error())
	}
	// written in place: /etc/hostname may be a bind mount (containers), which a rename would fail on
	if err := os.WriteFile(s.HostnameFile, []byte(name+"\n"), 0o644); err != nil {
		slog.Error("writing hostname file failed", "file", s.HostnameFile, "err", err)
		if rerr := s.SetHostname(previous); rerr != nil {
			slog.Error("restoring the previous hostname failed", "hostname", previous, "err", rerr)
		}
		return sdk.NewError(fiber.StatusInternalServerError, "hostname_write_failed", "could not write "+s.HostnameFile)
	}
	slog.Info("hostname changed", "hostname", name, "user", sdk.UserFrom(c).Name)
	return c.JSON(fiber.Map{"hostname": name})
}
