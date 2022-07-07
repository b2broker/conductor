package options

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type EmptyEnvError struct {
	option  string
	envName string
}

func (e *EmptyEnvError) Error() string {
	return fmt.Sprintf("%s required and can't be empty, set environment variable %s", e.option, e.envName)
}

type BadFormattedEnvError struct {
	option  string
	envName string
	format  string
}

func (e *BadFormattedEnvError) Error() string {
	return fmt.Sprintf("%s is bad formatted, set environment variable %s, formatted in: %s", e.option, e.envName, e.format)
}

type Option func(*options) error

type options struct {
	amqpDsn           *url.URL
	registryLogin     string
	registryPassword  string
	rpcQueue          string
	startupTimeout    time.Duration
	attachableNetwork string
	// TODO: should be removed
	dockerImage string
}

func AmqpDSN(envName string, def string) Option {
	return func(opts *options) error {
		value := def
		if env, ok := os.LookupEnv(envName); ok {
			value = env
		}
		url, err := url.Parse(value)
		if err != nil {
			return &BadFormattedEnvError{
				option:  "Amqp DSN",
				envName: envName,
				format:  "[scheme:][//[userinfo@]host][/]path",
			}
		}
		opts.amqpDsn = url
		return nil
	}
}

func RegistryLogin(envName string) Option {
	return func(opts *options) error {
		if env, ok := os.LookupEnv(envName); ok {
			opts.registryLogin = env
			return nil
		}
		return &EmptyEnvError{
			option:  "Registry Login",
			envName: envName,
		}
	}
}

func RegistryPassword(envName string) Option {
	return func(opts *options) error {
		if env, ok := os.LookupEnv(envName); ok {
			opts.registryPassword = env
			return nil
		}
		return &EmptyEnvError{
			option:  "Registry Password",
			envName: envName,
		}
	}
}

func RPCQueue(envName string, def string) Option {
	return func(opts *options) error {
		value := def
		if env, ok := os.LookupEnv(envName); ok {
			value = env
		}
		opts.rpcQueue = value
		return nil
	}
}

func AttachableNetwork(envName string) Option {
	return func(opts *options) error {
		if env, ok := os.LookupEnv(envName); ok {
			opts.attachableNetwork = env
			return nil
		}
		return &EmptyEnvError{
			option:  "Attachable Network",
			envName: envName,
		}
	}
}

func StartupTimeout(envName string, def int64) Option {
	return func(opts *options) error {
		value := def
		if env, ok := os.LookupEnv(envName); ok {
			val, err := strconv.ParseInt(env, 10, 64)
			if err != nil {
				return err
			}
			value = val
		}
		if value < 0 {
			value *= -1
		}
		opts.startupTimeout = time.Duration(value) * time.Second
		return nil
	}
}

func DockerImage(envName string) Option {
	return func(opts *options) error {
		if env, ok := os.LookupEnv(envName); ok {
			opts.dockerImage = env
			return nil
		}
		return &EmptyEnvError{
			option:  "Docker Image",
			envName: envName,
		}
	}
}

func (o *options) AmqpDSN() *url.URL {
	return o.amqpDsn
}

func (o *options) RegistryLogin() string {
	return o.registryLogin
}

func (o *options) RegistryPassword() string {
	return o.registryPassword
}

func (o *options) RPCQueue() string {
	return o.rpcQueue
}

func (o *options) AttachableNetwork() string {
	return o.attachableNetwork
}

func (o *options) StartupTimeout() time.Duration {
	return o.startupTimeout
}

func (o *options) DockerImage() string {
	return o.dockerImage
}

func Configure(opts ...Option) (*options, error) {
	config := &options{}
	for _, opt := range opts {
		if opt != nil {
			if err := opt(config); err != nil {
				return nil, err
			}
		}
	}
	return config, nil
}
