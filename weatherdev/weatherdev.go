// Copyright 2025 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weatherdev

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"cloudeng.io/webapi/clients/nws"
	"github.com/cosnicolaou/automation/devices"
)

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
		"forecasts": s.forecasts,
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
		"maxCloudCover": f.MaxOpacity,
	}
}

func (f *Forecast) ConditionsHelp() map[string]string {
	return map[string]string{
		"maxCloudCover": fmt.Sprintf("returns true if the cloud coverage is at most one of %v", argsValues),
	}
}

func (f *Forecast) CloudCoverage(ctx context.Context, opts devices.OperationArgs) (nws.OpaqueCloudCoverage, error) {
	if len(opts.Args) != 1 {
		return nws.UnknownOpaqueCloudCoverage, fmt.Errorf("expected a time argument in RFC3339 format")
	}
	when, err := time.Parse(time.RFC3339, opts.Args[0])
	if err != nil {
		return nws.UnknownOpaqueCloudCoverage, fmt.Errorf("failed to parse time: %v", err)
	}
	fc, err := f.service.Forecasts(ctx, opts)
	if err != nil {
		return nws.UnknownOpaqueCloudCoverage, err
	}
	p, ok := fc.PeriodFor(when)
	if !ok {
		return nws.UnknownOpaqueCloudCoverage, fmt.Errorf("no forecast available for current time")
	}
	return p.OpaqueCloudCoverage, nil
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

// Returns true if the cloud coverage is at most that specified by the argument.
func (f *Forecast) MaxOpacity(ctx context.Context, opts devices.OperationArgs) (bool, error) {
	if len(opts.Args) != 1 {
		return false, fmt.Errorf("expected an argument for cloud cover: one of %v", argsValues)
	}
	cc := opts.Args[0]
	atMost := nws.CloudOpacityFromShortForecast(cc)
	if atMost == nws.UnknownOpaqueCloudCoverage {
		return false, fmt.Errorf("unknown cloud cover: %q not one of %v", cc, argsValues)
	}
	opts.Args[0] = opts.Due.Format(time.RFC3339)
	opc, err := f.CloudCoverage(ctx, opts)
	if err != nil {
		return false, err
	}
	return opc <= atMost, nil
}
