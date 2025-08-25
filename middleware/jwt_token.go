package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Appy29/rate-limiter/utils"
	"github.com/golang-jwt/jwt/v4"
)

// JWT context keys
type jwtContextKey string

const (
	UserIDKey jwtContextKey = "user_id"
)

// JWTClaims represents the JWT payload
type JWTClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// JWTMiddleware validates JWT token and extracts user ID
func JWTMiddleware(jwtSecret string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			logger := utils.GetLoggerFromContext(r.Context())

			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn("Missing Authorization header")
				utils.SendError(w, http.StatusUnauthorized, "Authorization header required")
				return
			}

			// Check Bearer format
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				logger.Warn("Invalid Authorization header format")
				utils.SendError(w, http.StatusUnauthorized, "Authorization header must be 'Bearer <token>'")
				return
			}

			tokenString := tokenParts[1]

			// Parse and validate JWT
			token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					logger.Warn("Unexpected signing method", "method", token.Header["alg"])
					return nil, jwt.NewValidationError("invalid signing method", jwt.ValidationErrorSignatureInvalid)
				}
				return []byte(jwtSecret), nil
			})

			if err != nil {
				logger.Warn("JWT validation failed", "error", err.Error())
				utils.SendError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			// Extract claims
			if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
				if claims.UserID == "" {
					logger.Warn("Missing user_id in JWT claims")
					utils.SendError(w, http.StatusUnauthorized, "Invalid token claims")
					return
				}

				// Add user ID to context
				ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
				r = r.WithContext(ctx)

				logger.Info("JWT validated successfully", "user_id", claims.UserID)

				// Call next handler
				next(w, r)
			} else {
				logger.Warn("Invalid JWT claims")
				utils.SendError(w, http.StatusUnauthorized, "Invalid token claims")
				return
			}
		}
	}
}

// GetUserIDFromContext extracts user ID from context
func GetUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GenerateJWT creates a JWT token for testing purposes
func GenerateJWT(userID string, jwtSecret string) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(jwt.TimeFunc().Add(24 * 60 * 60 * 1000000000)), // 24 hours
			IssuedAt:  jwt.NewNumericDate(jwt.TimeFunc()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}
