package auth

import (
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
    "golang.org/x/crypto/bcrypt"
)

const (
    accessTokenDuration = 15 * time.Minute
    refreshTokenDuration = 7 * 24 * time.Hour
)

type AuthHandler struct {
    db        *pgxpool.Pool
    redis     *redis.Client
    secretKey []byte
}

func NewAuthHandler(db *pgxpool.Pool, redis *redis.Client, secret string) *AuthHandler {
    return &AuthHandler{
        db:        db,
        redis:     redis,
        secretKey: []byte(secret),
    }
}

func (h *AuthHandler) Register(c *gin.Context) {
    var req struct {
        Username string `json:"username" binding:"required,min=3,max=50"`
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=8"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
        return
    }
    
    _, err = h.db.Exec(context.Background(),
        "INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3)",
        req.Username, req.Email, string(hashedPassword))
    if err != nil {
        c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
        return
    }
    
    c.Status(http.StatusCreated)
}

func (h *AuthHandler) Login(c *gin.Context) {
    var req struct {
        Identifier string `json:"identifier" binding:"required"`
        Password   string `json:"password" binding:"required"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var user struct {
        ID           uuid.UUID
        PasswordHash string
    }
    
    err := h.db.QueryRow(context.Background(),
        "SELECT id, password_hash FROM users WHERE username = $1 OR email = $1",
        req.Identifier).Scan(&user.ID, &user.PasswordHash)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
        return
    }
    
    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
        return
    }
    
    accessToken, err := h.createToken(user.ID, accessTokenDuration)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
        return
    }
    
    refreshToken, err := h.createToken(user.ID, refreshTokenDuration)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "expires_in":    int(accessTokenDuration.Seconds()),
    })
}

func (h *AuthHandler) createToken(userID uuid.UUID, duration time.Duration) (string, error) {
    claims := jwt.MapClaims{
        "sub": userID,
        "exp": time.Now().Add(duration).Unix(),
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(h.secretKey)
}

func AuthMiddleware(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            return
        }
        
        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
        token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return []byte(secret), nil
        })
        
        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }
        
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
            return
        }
        
        userID, err := uuid.Parse(claims["sub"].(string))
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID"})
            return
        }
        
        c.Set("userID", userID)
        c.Next()
    }
}