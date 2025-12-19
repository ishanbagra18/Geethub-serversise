package middleware

import (
    "net/http"
    "strings"

    helper "github.com/ishanbagra18/ecommerce-using-go/helpers"
    "github.com/gin-gonic/gin"
)

// Use this for routes that REQUIRE login (e.g., /history/my)
func Authentication() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header missing"})
            c.Abort()
            return
        }

        parts := strings.Split(authHeader, " ")
        if len(parts) != 2 || parts[0] != "Bearer" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization format"})
            c.Abort()
            return
        }

        token := parts[1]
        claims, err := helper.ValidateToken(token)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
            c.Abort()
            return
        }

        c.Set("user_id", claims.Uid)
        c.Next()
    }
}

// Use this for routes that are PUBLIC but should track history if a user is logged in
func OptionalAuthentication() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.Next()
            return
        }

        parts := strings.Split(authHeader, " ")
        if len(parts) == 2 && parts[0] == "Bearer" {
            token := parts[1]
            claims, err := helper.ValidateToken(token)
            if err == nil {
                c.Set("user_id", claims.Uid)
            }
        }
        c.Next()
    }
}