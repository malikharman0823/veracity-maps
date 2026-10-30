package tests

import (
	"math"
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

func TestGISTransform_SimilarityConformalAtMidLatitude(t *testing.T) {
	// True north-up 1000x1000 square map from Google Maps / Web Mercator at ~51.51° N:
	// An equal ground/Mercator span has dLat = dLon * cos(meanLat).
	lonTL, latTL := -0.132000, 51.510000
	dLon := 0.008000
	cosLat := math.Cos(51.50751 * math.Pi / 180.0)
	dLat := dLon * cosLat
	lonBR := lonTL + dLon
	latBR := latTL - dLat

	points := []*models.ControlPoint{
		{ImageX: 0, ImageY: 0, Longitude: lonTL, Latitude: latTL},
		{ImageX: 999, ImageY: 999, Longitude: lonBR, Latitude: latBR},
	}

	result, err := gis.CalculateGeoreference(points, 1000, 1000, "similarity")
	if err != nil {
		t.Fatalf("unexpected similarity transform error: %v", err)
	}

	rotDeg := result.Parameters["rotation_degrees"]
	if math.Abs(rotDeg) > 0.05 {
		t.Fatalf("expected ~0 deg rotation for north-up Google Maps square, got %.4f deg", rotDeg)
	}

	// Verify top-right corner (999, 0) maps to (lonBR, latTL) without false rotation skew
	a, b, c := result.Transform[0], -result.Transform[1], result.Transform[2]
	d, e, f := result.Transform[3], -result.Transform[4], result.Transform[5]
	projTRLon := a*999 + b*0 + c
	projTRLat := d*999 + e*0 + f

	if math.Abs(projTRLon-lonBR) > 1e-6 || math.Abs(projTRLat-latTL) > 1e-6 {
		t.Fatalf("expected top-right corner (%.6f, %.6f), got (%.6f, %.6f)", lonBR, latTL, projTRLon, projTRLat)
	}

	// Verify both control points map back identically
	projBRlon := a*999 + b*(-999) + c
	projBRlat := d*999 + e*(-999) + f
	if math.Abs(projBRlon-lonBR) > 1e-9 || math.Abs(projBRlat-latBR) > 1e-9 {
		t.Fatalf("control point 2 mismatch: expected (%.8f, %.8f), got (%.8f, %.8f)", lonBR, latBR, projBRlon, projBRlat)
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
