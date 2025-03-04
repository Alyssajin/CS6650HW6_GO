package main

import (
	"database/sql"
	"log"
	"os"

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
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN environment variable not set")
	}

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	db.Exec(`
	CREATE TABLE IF NOT EXISTS test_table (
		id INT AUTO_INCREMENT PRIMARY KEY,
		some_value INT
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

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s ...", port)
	r.Run(":" + port)
}
