package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"
)

var (
    dbPool    *pgxpool.Pool
    redisClient *redis.Client
)

func main() {
    // Initialize connections
    initDB()
    initRedis()
    
    router := gin.Default()
    
    // Middleware
    router.Use(CORS())
    router.Use(rateLimiterMiddleware())
    
    // Routes
    router.GET("/health", healthCheck)
    
    // Server configuration
    srv := &http.Server{
        Addr:    ":8080",
        Handler: router,
    }
    
    // Graceful shutdown
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            panic(err)
        }
    }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        panic(err)
    }
    dbPool.Close()
    redisClient.Close()
}

func initDB() {
    poolConfig, err := pgxpool.ParseConfig(os.Getenv("DB_URL"))
    if err != nil {
        panic(err)
    }
    dbPool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
    if err != nil {
        panic(err)
    }
}

func initRedis() {
    opt, err := redis.ParseURL(os.Getenv("REDIS_URL"))
    if err != nil {
        panic(err)
    }
    redisClient = redis.NewClient(opt)
}

func healthCheck(c *gin.Context) {
    // Test DB connection
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    if err := dbPool.Ping(ctx); err != nil {
        c.JSON(503, gin.H{"status": "unhealthy", "error": err.Error()})
        return
    }
    
    // Test Redis connection
    if err := redisClient.Ping(ctx).Err(); err != nil {
        c.JSON(503, gin.H{"status": "unhealthy", "error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "status":   "healthy",
        "version":  "1.0.0",
        "postgres": dbPool.Stat().TotalConns(),
        "redis":    redisClient.PoolStats(),
    })
}

// CORS middleware configuration
func CORS() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
        c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        c.Writer.Header().Set("Access-Control-Max-Age", "86400")
        
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)
            return
        }
        
        c.Next()
    }
}

// Rate limiter middleware (basic implementation)
func rateLimiterMiddleware() gin.HandlerFunc {
    limiter := redis.NewLimiter(redisClient)
    return func(c *gin.Context) {
        if !limiter.Allow(c, "api", 100, time.Minute) {
            c.AbortWithStatusJSON(429, gin.H{"error": "Too many requests"})
            return
        }
        c.Next()
    }
}