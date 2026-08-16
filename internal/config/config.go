package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host      string          `json:"Host"`
	Port      string          `json:"Port"`
	Broker    BrokerConfig    `json:"Broker"`
	Providers ProvidersConfig `json:"Providers"`
	Templates TemplatesConfig `json:"Templates"`
}

type BrokerConfig struct {
	Host          string            `json:"Host"`
	User          string            `json:"User"`
	Password      string            `json:"Password"`
	PasswordEnv   string            `json:"PasswordEnv"`
	ExchangeName  string            `json:"ExchangeName"`
	ExchangeKind  string            `json:"ExchangeKind"`
	ConsumeQueue  string            `json:"ConsumeQueue"`
	PrefetchCount int               `json:"PrefetchCount"`
	RoutingKeys   BrokerRoutingKeys `json:"RoutingKeys"`
}

type BrokerRoutingKeys struct {
	DeliveryRequested string `json:"DeliveryRequested"`
	DeliveryResult    string `json:"DeliveryResult"`
}

type ProvidersConfig struct {
	Email ChannelConfig `json:"Email"`
	Phone PhoneConfig   `json:"Phone"`
	Push  ChannelConfig `json:"Push"`
}

type ChannelConfig struct {
	Enabled          bool                     `json:"Enabled"`
	DefaultProvider  string                   `json:"DefaultProvider"`
	AllowedProviders []string                 `json:"AllowedProviders"`
	Adapters         map[string]AdapterConfig `json:"Adapters"`
}

type PhoneConfig struct {
	DefaultProviderChain []string                 `json:"DefaultProviderChain"`
	AllowedProviders     []string                 `json:"AllowedProviders"`
	Adapters             map[string]AdapterConfig `json:"Adapters"`
}

type AdapterConfig map[string]any

func (a AdapterConfig) String(key string) string {
	if a == nil {
		return ""
	}
	value, _ := a[key].(string)
	return value
}

func (a AdapterConfig) EnvString(key string) string {
	envName := a.String(key)
	if envName == "" {
		return ""
	}
	return os.Getenv(envName)
}

func (a AdapterConfig) Bool(key string) bool {
	if a == nil {
		return false
	}
	value, _ := a[key].(bool)
	return value
}

func (a AdapterConfig) Int(key string) int {
	if a == nil {
		return 0
	}
	switch value := a[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case string:
		n, _ := strconv.Atoi(value)
		return n
	default:
		return 0
	}
}

type TemplatesConfig struct {
	DefaultLocale string                    `json:"DefaultLocale"`
	BaseDir       string                    `json:"BaseDir"`
	Items         map[string]TemplateConfig `json:"Items"`
}

type TemplateConfig struct {
	Subject           map[string]string `json:"Subject"`
	Body              map[string]string `json:"Body"`
	TextBody          map[string]string `json:"TextBody"`
	TextBodyFile      map[string]string `json:"TextBodyFile"`
	HtmlBody          map[string]string `json:"HtmlBody"`
	HtmlBodyFile      map[string]string `json:"HtmlBodyFile"`
	ContentType       string            `json:"ContentType"`
	RequiredVariables []string          `json:"RequiredVariables"`
}

func Load(path string) (*Config, error) {
	path = filepath.Clean(path)
	// #nosec G304 -- the config path is an operator-provided startup argument.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: decode %q: %w", path, err)
	}
	cfg.applyEnv()
	cfg.setDefaults()
	cfg.resolvePaths(path)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("MESSAGE_DELIVERY_HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv("MESSAGE_DELIVERY_PORT"); v != "" {
		c.Port = v
	}
	if v := os.Getenv("MESSAGE_DELIVERY_BROKER_HOST"); v != "" {
		c.Broker.Host = v
	}
	if v := os.Getenv("MESSAGE_DELIVERY_BROKER_USER"); v != "" {
		c.Broker.User = v
	}
	if c.Broker.PasswordEnv != "" {
		if v := os.Getenv(c.Broker.PasswordEnv); v != "" {
			c.Broker.Password = v
		}
	}
	if v := os.Getenv("MESSAGE_DELIVERY_BROKER_PASSWORD"); v != "" {
		c.Broker.Password = v
	}
	if v := os.Getenv("MESSAGE_DELIVERY_BROKER_PREFETCH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Broker.PrefetchCount = n
		}
	}
}

func (c *Config) setDefaults() {
	if c.Port == "" {
		c.Port = "8090"
	}
	if c.Broker.Host == "" {
		c.Broker.Host = "127.0.0.1:5672"
	}
	if c.Broker.User == "" {
		c.Broker.User = "guest"
	}
	if c.Broker.ExchangeName == "" {
		c.Broker.ExchangeName = "messages.events"
	}
	if c.Broker.ExchangeKind == "" {
		c.Broker.ExchangeKind = "topic"
	}
	if c.Broker.ConsumeQueue == "" {
		c.Broker.ConsumeQueue = "message.delivery.requests"
	}
	if c.Broker.PrefetchCount <= 0 {
		c.Broker.PrefetchCount = 8
	}
	if c.Broker.RoutingKeys.DeliveryRequested == "" {
		c.Broker.RoutingKeys.DeliveryRequested = "message.delivery.requested"
	}
	if c.Broker.RoutingKeys.DeliveryResult == "" {
		c.Broker.RoutingKeys.DeliveryResult = "message.delivery.result"
	}
	if c.Providers.Email.DefaultProvider == "" {
		c.Providers.Email.DefaultProvider = "fake-email"
	}
	if len(c.Providers.Email.AllowedProviders) == 0 {
		c.Providers.Email.AllowedProviders = []string{c.Providers.Email.DefaultProvider}
	}
	if len(c.Providers.Phone.DefaultProviderChain) == 0 {
		c.Providers.Phone.DefaultProviderChain = []string{"telegram", "sms"}
	}
	if len(c.Providers.Phone.AllowedProviders) == 0 {
		c.Providers.Phone.AllowedProviders = c.Providers.Phone.DefaultProviderChain
	}
	if c.Providers.Push.DefaultProvider == "" {
		c.Providers.Push.DefaultProvider = "webpush"
	}
	if len(c.Providers.Push.AllowedProviders) == 0 {
		c.Providers.Push.AllowedProviders = []string{c.Providers.Push.DefaultProvider}
	}
	if c.Templates.DefaultLocale == "" {
		c.Templates.DefaultLocale = "en"
	}
	if c.Templates.Items == nil {
		c.Templates.Items = map[string]TemplateConfig{}
	}
}

func (c *Config) resolvePaths(configPath string) {
	if c.Templates.BaseDir == "" || filepath.IsAbs(c.Templates.BaseDir) {
		return
	}
	c.Templates.BaseDir = filepath.Join(filepath.Dir(configPath), c.Templates.BaseDir)
}

func (c *Config) Validate() error {
	if c.Broker.Host == "" {
		return errors.New("config: Broker.Host must not be empty")
	}
	if c.Broker.ExchangeName == "" {
		return errors.New("config: Broker.ExchangeName must not be empty")
	}
	if c.Broker.ConsumeQueue == "" {
		return errors.New("config: Broker.ConsumeQueue must not be empty")
	}
	if c.Broker.RoutingKeys.DeliveryRequested == "" {
		return errors.New("config: Broker.RoutingKeys.DeliveryRequested must not be empty")
	}
	if c.Broker.RoutingKeys.DeliveryResult == "" {
		return errors.New("config: Broker.RoutingKeys.DeliveryResult must not be empty")
	}
	if len(c.Providers.Email.AllowedProviders) == 0 {
		return errors.New("config: Providers.Email.AllowedProviders must not be empty")
	}
	if len(c.Providers.Phone.AllowedProviders) == 0 {
		return errors.New("config: Providers.Phone.AllowedProviders must not be empty")
	}
	if len(c.Providers.Phone.DefaultProviderChain) == 0 {
		return errors.New("config: Providers.Phone.DefaultProviderChain must not be empty")
	}
	if len(c.Providers.Push.AllowedProviders) == 0 {
		return errors.New("config: Providers.Push.AllowedProviders must not be empty")
	}
	if len(c.Templates.Items) == 0 {
		return errors.New("config: Templates.Items must not be empty")
	}
	return nil
}

func (c *Config) BrokerURL() string {
	u := url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(c.Broker.User, c.Broker.Password),
		Host:   c.Broker.Host,
		Path:   "/",
	}
	return u.String()
}

func (c *Config) AllowedProvider(recipientType, provider string) bool {
	var allowed []string
	switch recipientType {
	case "email":
		allowed = c.Providers.Email.AllowedProviders
	case "phone":
		allowed = c.Providers.Phone.AllowedProviders
	case "push":
		allowed = c.Providers.Push.AllowedProviders
	default:
		return false
	}
	for _, candidate := range allowed {
		if candidate == provider {
			return true
		}
	}
	return false
}

func (c *Config) DefaultProviderChain(recipientType string) []string {
	switch recipientType {
	case "email":
		return []string{c.Providers.Email.DefaultProvider}
	case "phone":
		return append([]string(nil), c.Providers.Phone.DefaultProviderChain...)
	case "push":
		return []string{c.Providers.Push.DefaultProvider}
	default:
		return nil
	}
}
