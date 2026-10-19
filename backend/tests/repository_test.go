package tests

import (
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

func TestSessionRepository_Lifecycle(t *testing.T) {
	repo := repository.NewSessionRepository()

	// 1. Create Session
	sess := repo.CreateSession()
	if sess.ID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// 2. Get Session
	got, err := repo.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error getting session: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatalf("expected session ID %s, got %s", sess.ID, got.ID)
	}

	// 3. Insert and Retrieve Dataset
	datasetID := repository.GenerateDatasetID()
	ds := &models.Dataset{
		ID:                 datasetID,
		Filename:           "test.png",
		FileSize:           1024,
		ContentType:        "image/png",
		GeoreferenceStatus: "not_started",
		ExtractedMetadata:  map[string]interface{}{"width": 100, "height": 100},
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := repo.InsertDataset(sess.ID, ds); err != nil {
		t.Fatalf("failed inserting dataset: %v", err)
	}

	retrieved, err := repo.GetDataset(sess.ID, datasetID)
	if err != nil {
		t.Fatalf("failed retrieving dataset: %v", err)
	}
	if retrieved.Filename != "test.png" {
		t.Fatalf("expected filename 'test.png', got '%s'", retrieved.Filename)
	}

	// 4. Session Isolation (Another session cannot access dataset)
	otherSess := repo.CreateSession()
	if _, err := repo.GetDataset(otherSess.ID, datasetID); err != repository.ErrDatasetNotFound {
		t.Fatalf("expected ErrDatasetNotFound for other session, got %v", err)
	}

	// 5. Control Points
	cp := &models.ControlPoint{
		ImageX:    10,
		ImageY:    10,
		Longitude: -74.0,
		Latitude:  40.7,
	}
	if err := repo.InsertControlPoint(sess.ID, datasetID, cp); err != nil {
		t.Fatalf("failed inserting control point: %v", err)
	}

	points, err := repo.ListControlPoints(sess.ID, datasetID)
	if err != nil || len(points) != 1 {
		t.Fatalf("expected 1 control point, got %d (err: %v)", len(points), err)
	}

	// 6. Delete Dataset
	if err := repo.DeleteDataset(sess.ID, datasetID); err != nil {
		t.Fatalf("failed deleting dataset: %v", err)
	}
	if _, err := repo.GetDataset(sess.ID, datasetID); err != repository.ErrDatasetNotFound {
		t.Fatalf("expected dataset to be deleted, got err: %v", err)
	}
}
