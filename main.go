package main

import (
    "encoding/json"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/confluentinc/confluent-kafka-go/kafka"
)

type KafkaProducer struct {
    producer *kafka.Producer
    messages chan kafka.Message
    wg       sync.WaitGroup
    closed   bool
}

func NewKafkaProducer(brokers string) (*KafkaProducer, error) {
    p, err := kafka.NewProducer(&kafka.ConfigMap{
        "bootstrap.servers":         brokers,
        "linger.ms":                 5,
        "compression.type":          "gzip",
        "receive.message.max.bytes": 2000000000,
        "security.protocol":         "PLAINTEXT",
    })

    if err != nil {
        return nil, err
    }

    kp := &KafkaProducer{
        producer: p,
        messages: make(chan kafka.Message, 100),
    }

    kp.wg.Add(1)
    go kp.startProducing()

    return kp, nil
}

func (kp *KafkaProducer) startProducing() {
    defer kp.wg.Done()
    for msg := range kp.messages {
        if kp.closed {
            return
        }

        log.Printf("Sending message to topic %s: %s", *msg.TopicPartition.Topic, string(msg.Value))

        const maxRetries = 5
        for i := 0; i < maxRetries; i++ {
            err := kp.producer.Produce(&msg, nil)
            if err != nil {
                log.Printf("Error producing message to Kafka: %v", err)
                if i < maxRetries-1 {
                    log.Println("Retrying...")
                    time.Sleep(2 * time.Second)
                } else {
                    log.Println("Failed to send to Kafka after retries")
                }
            } else {
                log.Println("Message sent to Kafka successfully")
                break
            }
        }
    }
}

func (kp *KafkaProducer) Close() {
    kp.closed = true
    close(kp.messages)
    kp.wg.Wait()
    kp.producer.Close()
}

func main() {
    brokers := os.Getenv("KAFKA_BROKERS")
    defaultTopic := os.Getenv("KAFKA_TOPIC")
    hkTopic := os.Getenv("HK_TOPIC")

    kafkaProducer, err := NewKafkaProducer(brokers)
    if err != nil {
        log.Fatalf("Failed to create Kafka producer: %v", err)
    }
    defer kafkaProducer.Close()

    http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }

        var jsonData map[string]interface{}
        if err := json.NewDecoder(r.Body).Decode(&jsonData); err != nil {
            http.Error(w, "Invalid JSON", http.StatusBadRequest)
            return
        }

        log.Printf("Received HTTP POST request with data: %v", jsonData)

        // Determine which topic to send the message to
        topic := defaultTopic
        switch {
        case jsonData["site"] == "USSL":
            topic = hkTopic
        default:
            log.Println("No matching key found in JSON. Using default topic.")
            topic = defaultTopic
        }

        // Serialize the JSON data back to a byte array
        messageBytes, err := json.Marshal(jsonData)
        if err != nil {
            log.Printf("Failed to serialize JSON: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        // Create Kafka message
        msg := kafka.Message{
            TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
            Value:          messageBytes,
        }

        go func(message kafka.Message) {
            if !kafkaProducer.closed {
                kafkaProducer.messages <- message
            }
        }(msg)

        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Data sent to Kafka successfully"))
    })

    log.Println("Starting server on :8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}

