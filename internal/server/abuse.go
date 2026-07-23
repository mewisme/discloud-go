package server

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const rateLimitWindow = time.Minute

// allowRate increments Valkey key for kind+subject and returns whether under limit.
// limit <= 0 disables. Fail-closed on cache errors.
func (s *Server) allowRate(r *http.Request, kind, subject string, limit int) bool {
	if limit <= 0 {
		return true
	}
	key := fmt.Sprintf("discloud:rl:%s:%s", kind, subject)
	n, err := s.cache.Incr(r.Context(), key)
	if err != nil {
		s.log.Error("rate limit incr failed", "kind", kind, "error", err)
		return false
	}
	if n == 1 {
		if err := s.cache.Expire(r.Context(), key, rateLimitWindow); err != nil {
			s.log.Error("rate limit expire failed", "kind", kind, "error", err)
			return false
		}
	}
	return n <= int64(limit)
}

func (s *Server) requireUploadRate(w http.ResponseWriter, r *http.Request) bool {
	ip := s.clientIP(r)
	if !s.allowRate(r, "upload:ip", ip, s.rateLimitUploadPerMin) {
		writeJSONError(w, http.StatusTooManyRequests, "Upload rate limit exceeded")
		return false
	}
	if u, ok := s.sessionUser(r); ok {
		if !s.allowRate(r, "upload:user", u.ID, s.rateLimitUploadPerMin) {
			writeJSONError(w, http.StatusTooManyRequests, "Upload rate limit exceeded")
			return false
		}
	}
	return true
}

func (s *Server) requireDownloadRate(w http.ResponseWriter, r *http.Request) bool {
	if !s.allowRate(r, "download:ip", s.clientIP(r), s.rateLimitDownloadPerMin) {
		if r.URL.Query().Get("json") == "1" {
			writeJSONError(w, http.StatusTooManyRequests, "Download rate limit exceeded")
		} else {
			http.Error(w, "Download rate limit exceeded", http.StatusTooManyRequests)
		}
		return false
	}
	return true
}

func (s *Server) checkUserQuota(ctx context.Context, userID string, addBytes int64) error {
	if s.maxUserBytes <= 0 {
		return nil
	}
	stats, err := s.store.OwnerFileStats(ctx, userID, s.now().UTC(), 7*24*time.Hour)
	if err != nil {
		return err
	}
	openBytes, err := s.store.SumOpenUploadBytesByOwner(ctx, userID)
	if err != nil {
		return err
	}
	if stats.TotalBytes+openBytes+addBytes > s.maxUserBytes {
		return errQuotaExceeded
	}
	return nil
}

func (s *Server) checkAnonDailyQuota(r *http.Request, addBytes int64) error {
	ip := s.clientIP(r)
	day := s.now().UTC().Format("20060102")

	if s.maxAnonUploadsPerDay > 0 {
		key := fmt.Sprintf("discloud:anon:uploads:%s:%s", ip, day)
		n, err := s.cache.Incr(r.Context(), key)
		if err != nil {
			s.log.Error("anon upload quota incr failed", "error", err)
			return errQuotaUnavailable
		}
		if n == 1 {
			_ = s.cache.Expire(r.Context(), key, 48*time.Hour)
		}
		if n > int64(s.maxAnonUploadsPerDay) {
			return errQuotaExceeded
		}
	}

	if s.maxAnonBytesPerDay > 0 && addBytes > 0 {
		key := fmt.Sprintf("discloud:anon:bytes:%s:%s", ip, day)
		n, err := s.cache.IncrBy(r.Context(), key, addBytes)
		if err != nil {
			s.log.Error("anon bytes quota incr failed", "error", err)
			return errQuotaUnavailable
		}
		if n == addBytes {
			_ = s.cache.Expire(r.Context(), key, 48*time.Hour)
		}
		if n > s.maxAnonBytesPerDay {
			return errQuotaExceeded
		}
	}
	return nil
}

// peekAnonUploadQuota checks count without permanently consuming when create fails later.
// We consume on successful session create via checkAnonDailyQuota(addBytes=0 for create count only).
func (s *Server) requireQuotaForUpload(w http.ResponseWriter, r *http.Request, fileSize int64) bool {
	if u, ok := s.sessionUser(r); ok {
		if err := s.checkUserQuota(r.Context(), u.ID, fileSize); err != nil {
			if err == errQuotaExceeded {
				writeJSONError(w, http.StatusInsufficientStorage, "Storage quota exceeded")
				return false
			}
			s.log.Error("user quota check failed", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return false
		}
		return true
	}
	if s.maxAnonBytesPerDay > 0 && fileSize > s.maxAnonBytesPerDay {
		writeJSONError(w, http.StatusInsufficientStorage, "File exceeds anonymous daily byte limit")
		return false
	}
	// Anon: count toward daily uploads; bytes counted at complete.
	if err := s.checkAnonDailyQuota(r, 0); err != nil {
		if err == errQuotaExceeded {
			writeJSONError(w, http.StatusTooManyRequests, "Anonymous daily upload limit exceeded")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return false
	}
	return true
}

func (s *Server) requireAnonCompleteBytes(w http.ResponseWriter, r *http.Request, fileSize int64) bool {
	if _, ok := s.sessionUser(r); ok {
		return true
	}
	if err := s.checkAnonDailyQuota(r, fileSize); err != nil {
		if err == errQuotaExceeded {
			writeJSONError(w, http.StatusInsufficientStorage, "Anonymous daily byte limit exceeded")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return false
	}
	return true
}

var (
	errQuotaExceeded    = fmt.Errorf("quota exceeded")
	errQuotaUnavailable = fmt.Errorf("quota unavailable")
)

// allowAuth keeps the signup/signin limiter (fixed product constants, 15m window).
func (s *Server) allowAuth(r *http.Request, kind string) bool {
	key := "discloud:rl:" + kind + ":" + s.clientIP(r)
	n, err := s.cache.Incr(r.Context(), key)
	if err != nil {
		s.log.Error("rate limit incr failed", "error", err)
		return false
	}
	if n == 1 {
		if err := s.cache.Expire(r.Context(), key, authRateLimitWindow); err != nil {
			s.log.Error("rate limit expire failed", "error", err)
			return false
		}
	}
	return n <= authRateLimit
}
