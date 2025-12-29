package tmskafka

import (
	"log"
	"net"
	"strconv"

	"github.com/segmentio/kafka-go"
)

func EnsureTopics(addr string, topics []kafka.TopicConfig) {
	log.Println("🚀 Starting Kafka Topic Migration (segmentio/kafka-go)...")

	// 1. Connect to the Broker
	// We use Dial to get a raw connection for Admin operations
	conn, err := kafka.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Kafka at %s: %v", addr, err)
	}
	defer conn.Close()

	// 2. Ensure we are talking to the Controller
	// Creating topics is an administrative task that must be handled by the Controller.
	// In a single-node setup, the node is its own controller, but this is best practice.
	controller, err := conn.Controller()
	if err != nil {
		log.Fatalf("❌ Failed to get controller info: %v", err)
	}

	var controllerConn *kafka.Conn
	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))

	// If the controller address is different or DNS is tricky (Docker internal vs external),
	// we might need to fallback to our original connection if we are on localhost.
	// For local dev, usually 'conn' is sufficient if connected to the right port.
	if controllerAddr == addr {
		controllerConn = conn
	} else {
		// Try to dial the controller specifically
		// NOTE: In Docker setups, the controller might advertise an internal hostname (e.g. "kafka:9093")
		// which your host cannot reach. In that case, we assume our initial connection (localhost:9094)
		// points to the controller anyway.
		log.Printf("ℹ️  Controller detected at %s. Attempting to dial...", controllerAddr)
		controllerConn, err = kafka.Dial("tcp", controllerAddr)
		if err != nil {
			log.Printf("⚠️  Could not dial controller at %s (%v). Falling back to %s", controllerAddr, err, addr)
			controllerConn = conn
		} else {
			defer controllerConn.Close()
		}
	}

	// 3. Get Existing Topics
	partitions, err := controllerConn.ReadPartitions()
	if err != nil {
		log.Fatalf("❌ Failed to read partitions: %v", err)
	}

	existingTopics := make(map[string]bool)
	for _, p := range partitions {
		existingTopics[p.Topic] = true
	}

	// 4. Create Missing Topics
	var topicsToCreate []kafka.TopicConfig

	for _, t := range topics {
		if existingTopics[t.Topic] {
			log.Printf("⏩ Topic '%s' already exists. Skipping.", t.Topic)
			continue
		}
		log.Printf("🆕 Queuing topic creation: %s", t.Topic)
		topicsToCreate = append(topicsToCreate, t)
	}

	if len(topicsToCreate) > 0 {
		err = controllerConn.CreateTopics(topicsToCreate...)
		if err != nil {
			log.Fatalf("❌ Failed to create topics: %v", err)
		}
		log.Println("✅ Successfully created new topics!")
	} else {
		log.Println("✨ No new topics to create.")
	}
}
