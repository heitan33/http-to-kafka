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
    topic    string
    messages chan json.RawMessage
    wg       sync.WaitGroup
    closed   bool
}

func NewKafkaProducer(brokers string, topic string) (*KafkaProducer, error) {
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
        topic:    topic,
        messages: make(chan json.RawMessage, 100),
    }

    kp.wg.Add(1)
    go kp.startProducing()

    return kp, nil
}

func (kp *KafkaProducer) startProducing() {
    defer kp.wg.Done()
    for message := range kp.messages {
        if kp.closed {
            return
        }
        log.Printf("Received message to send to Kafka: %s", string(message))

        const maxRetries = 5
        for i := 0; i < maxRetries; i++ {
            err := kp.Produce(message)
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

func (kp *KafkaProducer) Produce(message json.RawMessage) error {
    msg := &kafka.Message{
        TopicPartition: kafka.TopicPartition{Topic: &kp.topic, Partition: kafka.PartitionAny},
        Value:          message,
    }

    deliveryChan := make(chan kafka.Event)
    defer close(deliveryChan)

    err := kp.producer.Produce(msg, deliveryChan)
    if err != nil {
        return err
    }

    e := <-deliveryChan
    m := e.(*kafka.Message)
    if m.TopicPartition.Error != nil {
        log.Printf("Delivery failed: %v\n", m.TopicPartition.Error)
        return m.TopicPartition.Error
    } else {
        log.Printf("Delivered message to %v\n", m.TopicPartition)
    }

    return nil
}

func (kp *KafkaProducer) Close() {
    kp.closed = true
    close(kp.messages)
    kp.wg.Wait()
    kp.producer.Close()
}

func main() {
    brokers := os.Getenv("KAFKA_BROKERS")
    topic := os.Getenv("KAFKA_TOPIC")

    kafkaProducer, err := NewKafkaProducer(brokers, topic)
    if err != nil {
        log.Fatalf("Failed to create Kafka producer: %v", err)
    }
    defer kafkaProducer.Close()

    http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }

        var jsonData json.RawMessage
        if err := json.NewDecoder(r.Body).Decode(&jsonData); err != nil {
            http.Error(w, "Invalid JSON", http.StatusBadRequest)
            return
        }

        log.Printf("Received HTTP POST request with data: %s", string(jsonData))

        go func(data json.RawMessage) {
            if !kafkaProducer.closed {
                kafkaProducer.messages <- data
            }
        }(jsonData)

        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Data sent to Kafka successfully"))
    })

    log.Println("Starting server on :8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}
