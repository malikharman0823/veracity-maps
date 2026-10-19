package tests

import (
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

func TestGISTransform_Similarity(t *testing.T) {
	// Image dimensions: 1000 x 800
	points := []*models.ControlPoint{
		{ImageX: 100, ImageY: 100, Longitude: -74.0, Latitude: 40.0},
		{ImageX: 900, ImageY: 700, Longitude: -73.0, Latitude: 41.0},
	}

	result, err := gis.CalculateGeoreference(points, 1000, 800, "similarity")
	if err != nil {
		t.Fatalf("unexpected similarity transform error: %v", err)
	}

	if result.Transformation != "similarity" {
		t.Fatalf("expected similarity transformation, got %s", result.Transformation)
	}
	if len(result.Transform) != 6 {
		t.Fatalf("expected 6 transform coefficients, got %d", len(result.Transform))
	}
	if len(result.MapLibreBounds) != 4 {
		t.Fatalf("expected 4 MapLibre bounds, got %d", len(result.MapLibreBounds))
	}
	if result.MapLibreBounds[0] >= result.MapLibreBounds[2] || result.MapLibreBounds[1] >= result.MapLibreBounds[3] {
		t.Fatalf("inverted MapLibre bounds: %v", result.MapLibreBounds)
	}
}

func TestGISTransform_Affine(t *testing.T) {
	// 4 non-collinear corner points
	points := []*models.ControlPoint{
		{ImageX: 50, ImageY: 50, Longitude: 10.0, Latitude: 50.0},
		{ImageX: 950, ImageY: 50, Longitude: 11.0, Latitude: 50.0},
		{ImageX: 950, ImageY: 750, Longitude: 11.0, Latitude: 49.0},
		{ImageX: 50, ImageY: 750, Longitude: 10.0, Latitude: 49.0},
	}

	result, err := gis.CalculateGeoreference(points, 1000, 800, "affine")
	if err != nil {
		t.Fatalf("unexpected affine transform error: %v", err)
	}

	if result.Transformation != "affine" {
		t.Fatalf("expected affine transformation, got %s", result.Transformation)
	}
	if len(result.MapLibreBounds) != 4 {
		t.Fatalf("expected 4 bounds values, got %d", len(result.MapLibreBounds))
	}
}

func TestGISTransform_CollinearRejection(t *testing.T) {
	// 3 points on a straight diagonal line across the image
	// (100, 100), (500, 400), (900, 700)
	points := []*models.ControlPoint{
		{ImageX: 100, ImageY: 100, Longitude: 10.0, Latitude: 50.0},
		{ImageX: 500, ImageY: 400, Longitude: 10.5, Latitude: 49.5},
		{ImageX: 900, ImageY: 700, Longitude: 11.0, Latitude: 49.0},
	}

	_, err := gis.CalculateGeoreference(points, 1000, 800, "auto")
	if err == nil {
		t.Fatalf("expected error for collinear points, got nil")
	}
	if !strings.Contains(err.Error(), "COLLINEAR_CONTROL_POINTS") {
		t.Fatalf("expected COLLINEAR_CONTROL_POINTS error, got: %v", err)
	}
}

func TestGISTransform_InsufficientPoints(t *testing.T) {
	points := []*models.ControlPoint{
		{ImageX: 100, ImageY: 100, Longitude: 10.0, Latitude: 50.0},
	}
	_, err := gis.CalculateGeoreference(points, 1000, 800, "auto")
	if err == nil || !strings.Contains(err.Error(), "INSUFFICIENT_CONTROL_POINTS") {
		t.Fatalf("expected INSUFFICIENT_CONTROL_POINTS, got: %v", err)
	}
}
