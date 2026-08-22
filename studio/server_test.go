package studio

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudioHandlerAllSections(t *testing.T) {
	handler := Handler(Options{
		DatabaseURL: "", // sample mock data
		SchemaPath:  "../examples/basic/default.asl",
	})

	// 1. Root page has sidebar with sections
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, section := range []string{"Types", "Queries", "Extensions", "Functions"} {
		if !strings.Contains(body, section) {
			t.Errorf("expected sidebar to contain %q section", section)
		}
	}
	if !strings.Contains(body, "id=\"panel-container\"") {
		t.Errorf("expected full page to contain panel-container")
	}

	// 2. Type view
	req = httptest.NewRequest(http.MethodGet, "/?kind=type&schema=public&table=user&tab=data", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET type view returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "email") {
		t.Errorf("expected type view to contain column 'email'")
	}

	// 3. HTMX SPA partial swap for panel-container
	req = httptest.NewRequest(http.MethodGet, "/?kind=type&schema=public&table=user&tab=structure", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "panel-container")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTMX partial swap returned %d", rec.Code)
	}
	spaBody := rec.Body.String()
	if !strings.Contains(spaBody, "Structure") || !strings.Contains(spaBody, "email") {
		t.Errorf("expected partial structure view content: %s", spaBody)
	}
	if strings.Contains(spaBody, "<html") || strings.Contains(spaBody, "id=\"sidebar-nav\"") {
		t.Errorf("expected partial view not to include layout or sidebar")
	}

	// 4. Extension view
	req = httptest.NewRequest(http.MethodGet, "/?kind=extension&name=pgcrypto", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET extension view returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pgcrypto") || !strings.Contains(rec.Body.String(), "use extension 'pgcrypto';") {
		t.Errorf("expected extension view to contain pgcrypto details: %s", rec.Body.String())
	}

	// 5. Function view
	req = httptest.NewRequest(http.MethodGet, "/?kind=function&name=slugify", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET function view returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "slugify") || !strings.Contains(rec.Body.String(), "PLPGSQL") {
		t.Errorf("expected function view to contain slugify details: %s", rec.Body.String())
	}

	// 6. Query view
	req = httptest.NewRequest(http.MethodGet, "/?kind=query&name=create_post", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET query view returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "create_post") {
		t.Errorf("expected query view to contain query name 'create_post'")
	}

	// 7. AQL post
	vals := url.Values{}
	vals.Set("aql", "multi select User { id, email } limit 10;")
	req = httptest.NewRequest(http.MethodPost, "/aql", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /aql returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "compiled SQL") && !strings.Contains(rec.Body.String(), "SELECT") {
		t.Errorf("expected compiled SQL in AQL response: %s", rec.Body.String())
	}
}

func TestStudioEnumSelectBox(t *testing.T) {
	// Create a temp schema with enum and type
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test_enum.asl")
	schemaContent := `
enum Status {
  active,
  pending,
  archived
}

type Membership {
  required id: uuid {
    default := gen_uuid();
    constraint pk;
  };
  required status: Status {
    default := 'active';
  };
}
`
	if err := os.WriteFile(schemaFile, []byte(schemaContent), 0644); err != nil {
		t.Fatal(err)
	}

	handler := Handler(Options{
		DatabaseURL: "",
		SchemaPath:  schemaFile,
	})

	req := httptest.NewRequest(http.MethodGet, "/?kind=type&schema=public&table=membership&tab=data", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "panel-container")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET membership returned %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-enum") {
		t.Errorf("expected enum select box with data-enum in response: %s", body)
	}
	if !strings.Contains(body, "pending") || !strings.Contains(body, "archived") {
		t.Errorf("expected enum options 'pending' and 'archived' in response: %s", body)
	}
}
