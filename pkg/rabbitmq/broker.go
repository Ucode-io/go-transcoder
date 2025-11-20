package rabbitmq

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/streadway/amqp"
	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
)

// RabbitMQ - structure that contains rabbit queue and channel
type RabbitMQ struct {
	Queues  map[string]amqp.Queue
	Channel *amqp.Channel
	Logger  logger.Logger
	Cfg     config.Config
}

// New - returns new RabbitMQ queue and channel
func New(cfg *config.Config, log logger.Logger) (*RabbitMQ, error) {
	log.Info(
		"Dialing to rabbitmq host with",
		logger.String("host", cfg.RabbitMqHost),
		logger.String("user", cfg.RabbitMqUser),
	)

	conn, err := amqp.Dial(
		fmt.Sprintf(
			"amqp://%s:%s@%s:%s/",
			cfg.RabbitMqUser,
			cfg.RabbitMqPassword,
			cfg.RabbitMqHost,
			cfg.RabbitMqPort,
		),
	)

	if err != nil {
		log.Error("Error while connecting to rabbitmq", logger.Error(err))
		return &RabbitMQ{}, err
	}

	log.Info("RabbitMQ connection is created...")

	channel, err := conn.Channel()
	if err != nil {
		log.Error("Error while connecting to channel", logger.Error(err))
		return &RabbitMQ{}, err
	}

	log.Info("RabbitMQ channel is created...")

	listen, err := channel.QueueDeclare(
		cfg.ListenQueue,
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		log.Error("Error while declaring queue", logger.Error(err))
		return &RabbitMQ{}, err
	}

	write, err := channel.QueueDeclare(
		cfg.WriteQueue,
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		log.Error("Error while declaring queue", logger.Error(err))
		return &RabbitMQ{}, err
	}

	err = channel.Qos(1, 0, false)
	if err != nil {
		log.Error("Error while setting Qos", logger.Error(err))
		return &RabbitMQ{}, err
	}

	return &RabbitMQ{
		Queues: map[string]amqp.Queue{
			cfg.ListenQueue: listen,
			cfg.WriteQueue:  write,
		},
		Channel: channel,
		Logger:  log,
		Cfg:     *cfg,
	}, nil
}

func (r *RabbitMQ) PublishPipelineStatus(req *models.UpdatePipelineStage) error {
	jsonByte, err := json.MarshalIndent(req, "", "    ")
	if err != nil {
		r.Logger.Error("Error while publishing new pipeline status")
		return err
	}

	err = r.Channel.Publish(
		"",
		r.Queues[r.Cfg.WriteQueue].Name,
		true,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        jsonByte,
		},
	)
	if err != nil {
		if strings.Contains(err.Error(), `"channel/connection is not open"`) {
			err = r.Reconnect()
			if err != nil {
				panic(`panic reason: "channel/connection is not open"`)
			} else {
				err = r.Channel.Publish(
					"",
					r.Queues[r.Cfg.WriteQueue].Name,
					true,
					false,
					amqp.Publishing{
						ContentType: "application/json",
						Body:        jsonByte,
					},
				)
			}
		}
		r.Logger.Error("Error while publishing the message", logger.Error(err))
		return err
	}

	return nil
}

func (r *RabbitMQ) Reconnect() error {
	r.Logger.Info("reconnecting to rabbitmq")

	conn, err := amqp.Dial(
		fmt.Sprintf(
			"amqp://%s:%s@%s:%s/",
			r.Cfg.RabbitMqUser,
			r.Cfg.RabbitMqPassword,
			r.Cfg.RabbitMqHost,
			r.Cfg.RabbitMqPort,
		),
	)

	if err != nil {
		r.Logger.Error("Error while connecting to rabbitmq", logger.Error(err))
		return err
	}

	r.Logger.Info("RabbitMQ connection is created...")

	r.Channel, err = conn.Channel()
	if err != nil {
		r.Logger.Error("Error while connecting to channel", logger.Error(err))
		return err
	}

	r.Logger.Info("RabbitMQ channel is created...")

	listen, err := r.Channel.QueueDeclare(
		r.Cfg.ListenQueue,
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		r.Logger.Error("Error while declaring queue", logger.Error(err))
		return err
	}

	write, err := r.Channel.QueueDeclare(
		r.Cfg.WriteQueue,
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		r.Logger.Error("Error while declaring queue", logger.Error(err))
		return err
	}

	r.Queues = map[string]amqp.Queue{
		r.Cfg.ListenQueue: listen,
		r.Cfg.WriteQueue:  write,
	}
	return nil
}
