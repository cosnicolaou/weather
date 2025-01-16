// Copyright 2025 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weatherdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"cloudeng.io/webapi/clients/nws"
	"github.com/cosnicolaou/automation/devices"
)

func NewController(typ string, opts devices.Options) (devices.Controller, error) {
	if typ == "weather.gov" {
		return NewService(opts), nil
	}
	return nil, fmt.Errorf("unsupported weather service %s", typ)
}

func NewDevice(typ string, opts devices.Options) (devices.Device, error) {
	if typ == "forecast" {
		return NewForecast(opts), nil
	}
	return nil, fmt.Errorf("unsupported weather device %s", typ)
}

func SupportedDevices() devices.SupportedDevices {
	return devices.SupportedDevices{
		"forecast": NewDevice,
	}
}

func SupportedControllers() devices.SupportedControllers {
	return devices.SupportedControllers{
		"weather.gov": NewController,
	}
}

type ServiceConfig struct {
	Refresh time.Duration `yaml:"refresh_interval"`
}

type Service struct {
	devices.ControllerBase[ServiceConfig]
	logger *slog.Logger

	mu  sync.Mutex
	api *nws.API
}

func NewService(opts devices.Options) *Service {
	c := &Service{
		logger: opts.Logger.With("protocol", "weather.gov"),
	}
	return c
}

func (s *Service) SetNWSAPI(api *nws.API) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.api = api
}

func (s *Service) Operations() map[string]devices.Operation {
	return map[string]devices.Operation{
		"forecast": s.forecasts,
	}
}

func (s *Service) OperationsHelp() map[string]string {
	return map[string]string{
		"forecast": "get the weather forecast for the system location that the controller belongs to",
	}
}

func (s *Service) forecasts(ctx context.Context, opts devices.OperationArgs) error {
	fc, err := s.Forecasts(ctx, opts)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	_, err = opts.Writer.Write(out)
	return err
}

func (s *Service) getAPI() *nws.API {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api == nil {
		s.api = nws.NewAPI(nws.WithForecastExpiration(s.ControllerConfigCustom.Refresh))
	}
	return s.api
}

func (s *Service) Forecasts(ctx context.Context, _ devices.OperationArgs) (nws.Forecast, error) {
	api := s.getAPI()
	loc := s.System().Location
	gp, err := api.LookupGridPoints(ctx, loc.Latitude, loc.Longitude)
	if err != nil {
		return nws.Forecast{}, err
	}
	fc, err := api.GetForecasts(ctx, gp)
	if err != nil {
		return nws.Forecast{}, err
	}
	return fc, nil
}

func (s *Service) Implementation() any {
	return s
}

type ForecastConfig struct{}

type Forecast struct {
	devices.DeviceBase[ForecastConfig]
	service *Service
}

func NewForecast(_ devices.Options) *Forecast {
	return &Forecast{}
}

func (f *Forecast) Implementation() any {
	return f
}

func (f *Forecast) SetController(c devices.Controller) {
	f.service = c.Implementation().(*Service)
}

func (f *Forecast) ControlledBy() devices.Controller {
	return f.service
}

func (f *Forecast) Conditions() map[string]devices.Condition {
	return map[string]devices.Condition{
		"cloud-cover":     f.Opacity,
		"max-cloud-cover": f.MaxOpacity,
		"min-cloud-cover": f.MinOpacity,
		"mostly-sunny":    f.MostlySunny,
		"partly-cloudy":   f.PartlyCloudy,
		"partly-sunny":    f.PartlyCloudy,
		"mostly-cloudy":   f.MostlyCloudy,
	}
}

func (f *Forecast) ConditionsHelp() map[string]string {
	return map[string]string{
		"cloud-cover":     "returns the cloud coverage at the current time",
		"max-cloud-cover": fmt.Sprintf("returns true if the cloud coverage is at most one of %v", argsValues),
		"min-cloud-cover": fmt.Sprintf("returns true if the cloud coverage is at least one of %v", argsValues),
		"mostly-sunny":    "returns true if the cloud coverage is at most mostly sunny",
		"partly-sunny":    "returns true if the cloud coverage is exactly partly sunny/cloudy",
		"partly-cloudy":   "returns true if the cloud coverage is exactly partly sunny/cloudy",
		"mostly-cloudy":   "returns true if the cloud coverage is at least mostly cloudy",
	}
}

var argsValues string

func init() {
	strs := []string{
		"Clear",
		"Sunny", // 0 to 1/8 Opaque Cloud Coverage
		"Mostly Clear",
		"Mostly Sunny", // 1/8 to 3/8
		"Partly Cloudy",
		"Partly Sunny",  // 3/8 to 5/8
		"Mostly Cloudy", // 5/8 to 7/8
		"Cloudy",        // 8/8
	}
	argsValues = strings.Join(strs, ", ")
}

func (f *Forecast) opacity(ctx context.Context, opts devices.OperationArgs) (forecast, wanted nws.OpaqueCloudCoverage, err error) {
	if len(opts.Args) != 1 {
		err = fmt.Errorf("expected an argument for cloud cover: one of %v", argsValues)
		return
	}
	wanted = nws.CloudOpacityFromShortForecast(opts.Args[0])
	if wanted == nws.UnknownOpaqueCloudCoverage {
		err = fmt.Errorf("unknown cloud cover: %q not one of %v", opts.Args[0], argsValues)
		return
	}
	fc, err := f.service.Forecasts(ctx, opts)
	if err != nil {
		return
	}
	if opts.Due.Equal(time.Time{}) {
		opts.Due = time.Now().In(f.service.System().Location.TimeLocation)
	}
	p, ok := fc.PeriodFor(opts.Due)
	if !ok {
		err = fmt.Errorf("no forecast available for time: %v", opts.Due)
		return
	}
	forecast = nws.CloudOpacityFromShortForecast(p.ShortForecast)
	if forecast == nws.UnknownOpaqueCloudCoverage {
		err = fmt.Errorf("unknown cloud cover in forecast: %q", p.ShortForecast)
		return
	}
	return forecast, wanted, nil
}

func (f *Forecast) writeMsg(wr io.Writer, msg string) {
	if wr != nil {
		_, _ = wr.Write([]byte(msg))
	}
}

// Opacity returns true if the cloud coverage is exactly that specified by the argument.
func (f *Forecast) Opacity(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	fc, arg, err := f.opacity(ctx, opts)
	if err != nil {
		return false, err
	}
	f.writeMsg(opts.Writer, fmt.Sprintf("Opacity: forecast: %v, wanted: %v == %v\n", fc, fc, arg))
	return fc == arg, nil
}

// MaxOpacity returns true if the cloud coverage is at most that specified by the argument.
func (f *Forecast) MaxOpacity(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	fc, arg, err := f.opacity(ctx, opts)
	if err != nil {
		return false, err
	}
	f.writeMsg(opts.Writer, fmt.Sprintf("MaxOpacity: forecast: %v, wanted: %v <= %v\n", fc, fc, arg))
	return fc <= arg, nil
}

// MinOpacity returns true if the cloud coverage is at most that specified by the argument.
func (f *Forecast) MinOpacity(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	fc, arg, err := f.opacity(ctx, opts)
	if err != nil {
		return false, err
	}
	f.writeMsg(opts.Writer, fmt.Sprintf("MinOpacity: forecast: %v, wanted: %v >= %v\n", fc, fc, arg))
	return fc >= arg, nil
}

func (f *Forecast) MostlySunny(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	opts.Args = []string{"Mostly Sunny"}
	return f.MaxOpacity(ctx, opts)
}

func (f *Forecast) PartlyCloudy(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	opts.Args = []string{"Partly Sunny"}
	return f.Opacity(ctx, opts)
}

func (f *Forecast) MostlyCloudy(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	opts.Args = []string{"Mostly Cloudy"}
	return f.MinOpacity(ctx, opts)
}
