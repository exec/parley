package queue

import (
	"fmt"
	"log"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange names
const (
	ExchangeMessages     = "parley.messages"
	ExchangeNotifications = "parley.notifications"
)

// Queue names
const (
	QueueMessageProcessing = "message.processing"
	QueueNotificationsSend = "notifications.send"
)

// Routing keys
const (
	RoutingKeyMessageCreate = "message.create"
	RoutingKeyMessageUpdate = "message.update"
	RoutingKeyMessageDelete = "message.delete"
	RoutingKeyNotification  = "notification"
)

// RabbitMQ holds the connection and channel
type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	mu      sync.Mutex
}

// NewRabbitMQ creates a new RabbitMQ connection
func NewRabbitMQ(url string) (*RabbitMQ, error) {
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchanges
	if err := declareExchanges(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	// Declare queues
	if err := declareQueues(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	// Bind queues to exchanges
	if err := bindQueues(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	log.Println("Connected to RabbitMQ")

	return &RabbitMQ{
		conn:    conn,
		channel: ch,
	}, nil
}

// declareExchanges declares the required exchanges
func declareExchanges(ch *amqp.Channel) error {
	// Topic exchange for messages
	err := ch.ExchangeDeclare(
		ExchangeMessages, // name
		"topic",           // type
		true,              // durable
		false,             // auto-deleted
		false,             // internal
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", ExchangeMessages, err)
	}

	// Fanout exchange for notifications
	err = ch.ExchangeDeclare(
		ExchangeNotifications, // name
		"fanout",              // type
		true,                  // durable
		false,                 // auto-deleted
		false,                 // internal
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", ExchangeNotifications, err)
	}

	return nil
}

// declareQueues declares the required queues
func declareQueues(ch *amqp.Channel) error {
	// Message processing queue
	_, err := ch.QueueDeclare(
		QueueMessageProcessing, // name
		true,                   // durable
		false,                  // delete when unused
		false,                  // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", QueueMessageProcessing, err)
	}

	// Notifications queue
	_, err = ch.QueueDeclare(
		QueueNotificationsSend, // name
		true,                   // durable
		false,                  // delete when unused
		false,                  // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", QueueNotificationsSend, err)
	}

	return nil
}

// bindQueues binds queues to exchanges
func bindQueues(ch *amqp.Channel) error {
	// Bind message processing queue to messages exchange with multiple routing keys
	err := ch.QueueBind(
		QueueMessageProcessing, // queue name
		RoutingKeyMessageCreate, // routing key
		ExchangeMessages,        // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", QueueMessageProcessing, err)
	}

	err = ch.QueueBind(
		QueueMessageProcessing,
		RoutingKeyMessageUpdate,
		ExchangeMessages,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", QueueMessageProcessing, err)
	}

	err = ch.QueueBind(
		QueueMessageProcessing,
		RoutingKeyMessageDelete,
		ExchangeMessages,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", QueueMessageProcessing, err)
	}

	// Bind notifications queue to notifications exchange
	err = ch.QueueBind(
		QueueNotificationsSend, // queue name
		"",                    // routing key (ignored for fanout)
		ExchangeNotifications,  // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", QueueNotificationsSend, err)
	}

	return nil
}

// Publish publishes a message to an exchange
func (r *RabbitMQ) Publish(exchange, routingKey string, body []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channel == nil {
		return fmt.Errorf("channel is closed")
	}

	err := r.channel.Publish(
		exchange,   // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Consume consumes messages from a queue
func (r *RabbitMQ) Consume(queue string, handler func([]byte)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channel == nil {
		return fmt.Errorf("channel is closed")
	}

	msgs, err := r.channel.Consume(
		queue, // queue
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	// Process messages in a goroutine
	go func() {
		for msg := range msgs {
			handler(msg.Body)
			// Acknowledge the message
			if err := msg.Ack(false); err != nil {
				log.Printf("Failed to acknowledge message: %v", err)
			}
		}
	}()

	return nil
}

// Close closes the RabbitMQ connection
func (r *RabbitMQ) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channel != nil {
		r.channel.Close()
		r.channel = nil
	}

	if r.conn != nil {
		err := r.conn.Close()
		if err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
		r.conn = nil
	}

	log.Println("RabbitMQ connection closed")
	return nil
}