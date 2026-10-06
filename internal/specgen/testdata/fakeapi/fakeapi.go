// Package fakeapi mirrors apiv3's shape for specgen's tests: a Client whose
// services take the widget fixture's params and return its model.
package fakeapi

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"time"

	"github.com/oapi-codegen/nullable"
)

// ErrNotFound mirrors apiv3.ErrNotFound, so the generated wrapper compiles
// against this package.
var ErrNotFound = errors.New("not found")

type Shape string

type GearKind string

type Mesh struct {
	Teeth int     `json:"teeth"`
	Pitch float64 `json:"pitch"`
}

type Gear struct {
	Kind GearKind `json:"kind"`
	Mesh Mesh     `json:"mesh"`
}

type Widget struct {
	Id        string                      `json:"id"`
	ProjectId string                      `json:"project_id"`
	Name      string                      `json:"name"`
	Shape     Shape                       `json:"shape"`
	Size      nullable.Nullable[int]      `json:"size"`
	Color     nullable.Nullable[string]   `json:"color"`
	Tags      []string                    `json:"tags"`
	Badges    []string                    `json:"badges"`
	Period    nullable.Nullable[string]   `json:"period"`
	Serial    *string                     `json:"serial"`
	Token     string                      `json:"token"`
	CreatedAt time.Time                   `json:"created_at"`
	Gear      nullable.Nullable[Gear]     `json:"gear"`
	Parts     nullable.Nullable[[]string] `json:"parts"`
}

// WidgetPartial lacks color, to test a model missing a field.
type WidgetPartial struct {
	Id string `json:"id"`
}

type WidgetCreateParams struct {
	Name   string                    `json:"name"`
	Shape  *Shape                    `json:"shape,omitempty"`
	Size   nullable.Nullable[int]    `json:"size,omitempty"`
	Color  nullable.Nullable[string] `json:"color,omitempty"`
	Tags   []string                  `json:"tags,omitempty"`
	Badges []string                  `json:"badges,omitempty"`
	Period nullable.Nullable[string] `json:"period,omitempty"`
	Secret nullable.Nullable[string] `json:"secret,omitempty"`
	Serial *string                   `json:"serial,omitempty"`
	Gear   Gear                      `json:"gear"`
	Parts  *[]string                 `json:"parts,omitempty"`
}

// WidgetBadCreateParams has serial as a plain value, which isn't Required.
type WidgetBadCreateParams struct {
	Name   string                    `json:"name"`
	Shape  *Shape                    `json:"shape,omitempty"`
	Size   nullable.Nullable[int]    `json:"size,omitempty"`
	Color  nullable.Nullable[string] `json:"color,omitempty"`
	Period nullable.Nullable[string] `json:"period,omitempty"`
	Secret nullable.Nullable[string] `json:"secret,omitempty"`
	Serial string                    `json:"serial"`
	Gear   Gear                      `json:"gear"`
	Parts  *[]string                 `json:"parts,omitempty"`
}

// WidgetPointerUpdateParams has color as a pointer, which can't send null.
type WidgetPointerUpdateParams struct {
	Name   *string                     `json:"name,omitempty"`
	Shape  *Shape                      `json:"shape,omitempty"`
	Size   nullable.Nullable[int]      `json:"size,omitempty"`
	Color  *string                     `json:"color,omitempty"`
	Period nullable.Nullable[string]   `json:"period,omitempty"`
	Secret nullable.Nullable[string]   `json:"secret,omitempty"`
	Gear   *Gear                       `json:"gear,omitempty"`
	Parts  nullable.Nullable[[]string] `json:"parts,omitempty"`
}

// WidgetEnumCreateParams has shape as a plain enum value, which isn't Required.
type WidgetEnumCreateParams struct {
	Name   string                    `json:"name"`
	Shape  Shape                     `json:"shape"`
	Size   nullable.Nullable[int]    `json:"size,omitempty"`
	Color  nullable.Nullable[string] `json:"color,omitempty"`
	Period nullable.Nullable[string] `json:"period,omitempty"`
	Secret nullable.Nullable[string] `json:"secret,omitempty"`
	Serial *string                   `json:"serial,omitempty"`
	Gear   Gear                      `json:"gear"`
	Parts  *[]string                 `json:"parts,omitempty"`
}

type WidgetUpdateParams struct {
	Name   *string                     `json:"name,omitempty"`
	Shape  *Shape                      `json:"shape,omitempty"`
	Size   nullable.Nullable[int]      `json:"size,omitempty"`
	Color  nullable.Nullable[string]   `json:"color,omitempty"`
	Tags   []string                    `json:"tags,omitempty"`
	Badges []string                    `json:"badges,omitempty"`
	Period nullable.Nullable[string]   `json:"period,omitempty"`
	Secret nullable.Nullable[string]   `json:"secret,omitempty"`
	Gear   *Gear                       `json:"gear,omitempty"`
	Parts  nullable.Nullable[[]string] `json:"parts,omitempty"`
}

// WidgetSlicePointerUpdateParams has parts as a pointer, which can't send
// null; a test makes parts removable to check that's refused.
type WidgetSlicePointerUpdateParams struct {
	Name   *string                   `json:"name,omitempty"`
	Shape  *Shape                    `json:"shape,omitempty"`
	Size   nullable.Nullable[int]    `json:"size,omitempty"`
	Color  nullable.Nullable[string] `json:"color,omitempty"`
	Period nullable.Nullable[string] `json:"period,omitempty"`
	Secret nullable.Nullable[string] `json:"secret,omitempty"`
	Gear   *Gear                     `json:"gear,omitempty"`
	Parts  *[]string                 `json:"parts,omitempty"`
}

// WidgetMapGearUpdateParams has gear as a map, which specgen doesn't map.
type WidgetMapGearUpdateParams struct {
	Name   *string                     `json:"name,omitempty"`
	Shape  *Shape                      `json:"shape,omitempty"`
	Size   nullable.Nullable[int]      `json:"size,omitempty"`
	Color  nullable.Nullable[string]   `json:"color,omitempty"`
	Period nullable.Nullable[string]   `json:"period,omitempty"`
	Secret nullable.Nullable[string]   `json:"secret,omitempty"`
	Gear   map[string]any              `json:"gear,omitempty"`
	Parts  nullable.Nullable[[]string] `json:"parts,omitempty"`
}

type Client struct {
	Gadgets         *GadgetsService
	Sprockets       *SprocketsService
	Widgets         *WidgetsService
	WidgetsSwapped  *WidgetsSwappedService
	WidgetsNoDelete *WidgetsNoDeleteService
}

type WidgetsService struct{}

func (s *WidgetsService) Create(ctx context.Context, projectID string, p WidgetCreateParams) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsService) Get(ctx context.Context, projectID, widgetID string) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsService) Update(ctx context.Context, projectID, widgetID string, p WidgetUpdateParams) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsService) Delete(ctx context.Context, projectID, widgetID string) error { return nil }

// WidgetsSwappedService takes its path parameters in the wrong order.
type WidgetsSwappedService struct{}

func (s *WidgetsSwappedService) Create(ctx context.Context, projectID string, p WidgetCreateParams) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsSwappedService) Get(ctx context.Context, widgetID, projectID string) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsSwappedService) Update(ctx context.Context, projectID, widgetID string, p WidgetUpdateParams) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsSwappedService) Delete(ctx context.Context, projectID, widgetID string) error {
	return nil
}

// WidgetsNoDeleteService has no Delete.
type WidgetsNoDeleteService struct{}

func (s *WidgetsNoDeleteService) Create(ctx context.Context, projectID string, p WidgetCreateParams) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsNoDeleteService) Get(ctx context.Context, projectID, widgetID string) (*Widget, error) {
	return nil, nil
}
func (s *WidgetsNoDeleteService) Update(ctx context.Context, projectID, widgetID string, p WidgetUpdateParams) (*Widget, error) {
	return nil, nil
}

type GadgetType string

type GadgetEvent string

type GadgetFilterEvent string

type GadgetFilter struct {
	Event GadgetFilterEvent `json:"event"`
	Query string            `json:"query"`
}

type GadgetLinks struct {
	Web string `json:"web"`
}

type Gadget struct {
	Id        string                 `json:"id"`
	ProjectId string                 `json:"project_id"`
	Type      string                 `json:"type"`
	Active    bool                   `json:"active"`
	Events    []string               `json:"events"`
	SpotIds   []uuid.UUID            `json:"spot_ids"`
	Filters   []GadgetFilter         `json:"filters"`
	Config    map[string]interface{} `json:"config"`
	Links     GadgetLinks            `json:"links"`
}

type GadgetCreateParams struct {
	Type    GadgetType                        `json:"type"`
	Active  *bool                             `json:"active,omitempty"`
	Events  nullable.Nullable[[]GadgetEvent]  `json:"events,omitempty"`
	SpotIds nullable.Nullable[[]uuid.UUID]    `json:"spot_ids,omitempty"`
	Filters nullable.Nullable[[]GadgetFilter] `json:"filters,omitempty"`
	Config  *map[string]interface{}           `json:"config,omitempty"`
}

type GadgetUpdateParams struct {
	Active  *bool                             `json:"active,omitempty"`
	Events  nullable.Nullable[[]GadgetEvent]  `json:"events,omitempty"`
	SpotIds nullable.Nullable[[]uuid.UUID]    `json:"spot_ids,omitempty"`
	Filters nullable.Nullable[[]GadgetFilter] `json:"filters,omitempty"`
	Config  *map[string]interface{}           `json:"config,omitempty"`
}

type GadgetsService struct{}

func (s *GadgetsService) Create(ctx context.Context, projectID string, p GadgetCreateParams) (*Gadget, error) {
	return nil, nil
}
func (s *GadgetsService) Get(ctx context.Context, projectID, gadgetID string) (*Gadget, error) {
	return nil, nil
}
func (s *GadgetsService) Update(ctx context.Context, projectID, gadgetID string, p GadgetUpdateParams) (*Gadget, error) {
	return nil, nil
}
func (s *GadgetsService) Delete(ctx context.Context, projectID, gadgetID string) error { return nil }

// Sprocket is the phase-5 fixture: an account-level resource with a UUID id.
type Sprocket struct {
	Id       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	ParentId uuid.UUID `json:"parent_id"`
}

type SprocketCreateParams struct {
	Name     string    `json:"name"`
	ParentId uuid.UUID `json:"parent_id"`
}

type SprocketUpdateParams struct {
	Name *string `json:"name,omitempty"`
}

type SprocketsService struct{}

func (s *SprocketsService) Create(ctx context.Context, p SprocketCreateParams) (*Sprocket, error) {
	return nil, nil
}
func (s *SprocketsService) Get(ctx context.Context, sprocketID uuid.UUID) (*Sprocket, error) {
	return nil, nil
}
func (s *SprocketsService) Update(ctx context.Context, sprocketID uuid.UUID, p SprocketUpdateParams) (*Sprocket, error) {
	return nil, nil
}
func (s *SprocketsService) Delete(ctx context.Context, sprocketID uuid.UUID) error { return nil }
