package tmskafka

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/segmentio/kafka-go"
)

// CommonProducer содержит общую логику для всех сервисов
type CommonProducer struct {
	Writer *kafka.Writer
}

func NewCommonProducer(addr, topic string, partitions, replications int) (*CommonProducer, error) {
	// Выносим EnsureTopicExists в этот же пакет (см. ниже)
	if err := EnsureTopicExists(addr, topic, partitions, replications); err != nil {
		return nil, err
	}

	writer := &kafka.Writer{
		Addr:     kafka.TCP(addr),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
		// Здесь можно добавить общие настройки: ReadTimeout, MaxAttempts и т.д.
	}

	return &CommonProducer{Writer: writer}, nil
}

// WriteRaw отправляет готовые байты. Это "фундамент" для типизированных методов.
func (p *CommonProducer) WriteRaw(ctx context.Context, key []byte, value []byte) error {
	return p.Writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}

func (p *CommonProducer) Close() error {
	return p.Writer.Close()
}

// EnsureTopicExists создает топик, если его нет
func EnsureTopicExists(addr, topic string, partitions, replications int) error {
	// Подключаемся к брокеру (tcp)
	conn, err := kafka.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Получаем список партиций (проверяем, есть ли топик)
	existingPartitions, err := conn.ReadPartitions()
	if err == nil {
		for _, p := range existingPartitions {
			if p.Topic == topic {
				return nil // Топик уже есть, всё ок
			}
		}
	}

	// Если топика нет — пытаемся создать
	// Нужно подключиться к Контроллеру (управляющему узлу)
	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	var controllerConn *kafka.Conn
	// Подключаемся к контроллеру
	controllerConn, err = kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfig := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,   // Для дева 1, для прода 3+
		ReplicationFactor: replications, // Для локали 1, для прода 3
	}

	err = controllerConn.CreateTopics(topicConfig)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	fmt.Printf("Auto-created Kafka topic: %s\n", topic)
	return nil
}
