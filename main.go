package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/auto/sdk"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	// "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	db     *sql.DB
	tracer trace.Tracer
)

// We'll store a global *sql.DB for simplicity in a demo.

type Album struct {
	Artist    string
	Title     string
	Year      int
	Image     []byte
	ImageSize int64
}

type Profile struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Year   string `json:"year"`
}

func main() {
	// Initialize OpenTelemetry
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}

	meterProvider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(meterProvider)

	tracerProvider := sdk.TracerProvider()
	otel.SetTracerProvider(tracerProvider)
	tracer = otel.Tracer("cs6650hw6_go")

	// Read MySQL DSN from environment variable
	// Read MySQL DSN from environment variable, e.g.:
	// DB_DSN = "mydbuser:mydbpass123@tcp(demo-mysql.ctmce9oladmi.us-west-2.rds.amazonaws.com:3306)/mydemodb"
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN environment variable not set")
	}

	// db.SetMaxOpenConns(80)
	// db.SetMaxIdleConns(25)
	// db.SetConnMaxLifetime(5 * time.Minute)

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	db.Exec(`
        CREATE TABLE IF NOT EXISTS album (
                id INT AUTO_INCREMENT PRIMARY KEY,
                artist VARCHAR(255) NOT NULL,
                title VARCHAR(255) NOT NULL,
                year INT NOT NULL,
                image LONGBLOB NOT NULL,
                image_size INT NOT NULL
        ) ENGINE=InnoDB;
        `)

	r := gin.Default()

	// Health check route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// GET /count -> returns row count in "test_table"
	r.GET("/count", func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), "GET /count")
		defer span.End()

		var cnt int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_table")
		if err := row.Scan(&cnt); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"row_count": cnt})
	})

	r.GET("/album/:albumId", func(c *gin.Context) {
		albumId := c.Param("albumId")
		id, err := strconv.Atoi(albumId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid album value"})
		}

		var albumObj Album
		row := db.QueryRow("SELECT artist, title, year FROM album WHERE id = ?", id)
		if err := row.Scan(&albumObj.Artist, &albumObj.Title, &albumObj.Year); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Album not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		c.JSON(200, albumObj)
	})

	// POST /insert -> inserts a row with some random value
	r.POST("/insert", func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), "POST /insert")
		defer span.End()

		res, err := db.ExecContext(ctx, "INSERT INTO test_table (some_value) VALUES (FLOOR(RAND()*1000))")
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		c.JSON(200, gin.H{"message": "inserted", "row_id": id})
	})

	r.POST("/album", func(c *gin.Context) {
		profileStr := c.PostForm("profile")
		if profileStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing profile field"})
			return
		}

		var profile Profile
		if err := json.Unmarshal([]byte(profileStr), &profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid profile JSON"})
			return
		}

		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing image file"})
			return
		}

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open image file"})
			return
		}
		defer src.Close()

		imageData, err := io.ReadAll(src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read image file"})
			return
		}
		imageSize := len(imageData)

		yearInt, err := strconv.Atoi(profile.Year)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year value"})
			return
		}

		res, err := db.Exec(`
                INSERT INTO album (artist, title, year, image, image_size) 
                VALUES (?, ?, ?, ?, ?)`,
			profile.Artist, profile.Title, yearInt, imageData, imageSize)

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		c.JSON(200, gin.H{"message": "Album created", "albumId": id, "imageSize": imageSize})
	})

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s ...", port)
	r.Run(":" + port)
}
