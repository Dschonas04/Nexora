package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Eine Adresse ohne Port wird abgewiesen und nicht geraten: ein geratener Port
// meldet einen Rechner als still, der nur auf einem anderen hoert.
func TestZielPruefen(t *testing.T) {
	faelle := []struct {
		wert string
		gut  bool
	}{
		{"10.0.0.5:22", true},
		{"nas.fritz.box:445", true},
		{"http://10.0.0.5:9090", true},
		{"https://beispiel.de", true},
		{"10.0.0.5", false},
		{"", false},
		{"   ", false},
		{"10.0.0.5:0", false},
		{"10.0.0.5:70000", false},
		{"10.0.0.5:ssh", false},
		{"ssh://10.0.0.5:22", false},
	}
	for _, f := range faelle {
		_, err := zielPruefen(f.wert)
		if f.gut && err != nil {
			t.Errorf("%q sollte durchgehen, kam: %v", f.wert, err)
		}
		if !f.gut && err == nil {
			t.Errorf("%q sollte abgewiesen werden", f.wert)
		}
	}
}

// Der Port faellt weg, weil Prometheus den des Exporters fuehrt (9100) und die
// Liste den, an dem angeklopft wird (22). Ohne das faende nichts zusammen.
func TestInstanzSchluessel(t *testing.T) {
	faelle := map[string]string{
		"10.0.0.5:22":            "10.0.0.5",
		"10.0.0.5:9100":          "10.0.0.5",
		"http://10.0.0.5:9090":   "10.0.0.5",
		"https://Nas.Fritz.Box/": "nas.fritz.box",
		"nas":                    "nas",
		"":                       "",
	}
	for ein, erwartet := range faelle {
		if raus := instanzSchluessel("", ein); raus != erwartet {
			t.Errorf("%q ergab %q, erwartet %q", ein, raus, erwartet)
		}
	}
	// Die eingetragene Kennung schlaegt das Ziel.
	if raus := instanzSchluessel("wirt-7:9100", "10.0.0.5:22"); raus != "wirt-7" {
		t.Errorf("Kennung nicht bevorzugt: %q", raus)
	}
}

// Anklopfen misst an einem offenen Port und an einem geschlossenen, damit beide
// Zweige einmal wirklich gelaufen sind.
func TestAnklopfen(t *testing.T) {
	horcher, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer horcher.Close()
	go func() {
		for {
			c, err := horcher.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if da, _, hinweis := anklopfen(context.Background(), horcher.Addr().String()); !da {
		t.Fatalf("offener Port gilt als still: %s", hinweis)
	}

	// Ein Port, auf dem nichts horcht: derselbe Horcher, nachdem er zu ist.
	zu := horcher.Addr().String()
	horcher.Close()
	if da, _, _ := anklopfen(context.Background(), zu); da {
		t.Fatal("geschlossener Port gilt als erreichbar")
	}
}

// Eine HTTP-Adresse zaehlt als erreichbar, auch wenn der Weg 404 gibt: gefragt
// ist, ob dort ein Dienst laeuft, nicht ob er diesen einen Pfad kennt.
func TestAnklopfenHTTPMitFehlerstatus(t *testing.T) {
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer dienst.Close()

	da, _, hinweis := anklopfen(context.Background(), dienst.URL)
	if !da {
		t.Fatal("404 gilt als still")
	}
	if hinweis == "" {
		t.Fatal("der Status fehlt im Hinweis")
	}
}

// Ein selbst unterschriebenes Zertifikat ist im eigenen Haus die Regel und darf
// den Rechner nicht als still melden: Proxmox, NAS und Backup-Server tragen
// alle eines.
func TestAnklopfenNimmtSelbstUnterschriebenesZertifikat(t *testing.T) {
	dienst := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dienst.Close()

	if da, _, hinweis := anklopfen(context.Background(), dienst.URL); !da {
		t.Fatalf("selbst unterschriebenes HTTPS gilt als still: %s", hinweis)
	}
}

// Einer Umleitung wird nicht gefolgt: sonst stuende am Ende die Erreichbarkeit
// eines ganz anderen Rechners in der Zeile.
func TestAnklopfenFolgtKeinerUmleitung(t *testing.T) {
	besucht := 0
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		besucht++
		http.Redirect(w, r, "/woanders", http.StatusMovedPermanently)
	}))
	defer dienst.Close()

	da, _, _ := anklopfen(context.Background(), dienst.URL)
	if !da {
		t.Fatal("eine Umleitung ist auch eine Antwort")
	}
	if besucht != 1 {
		t.Fatalf("der Umleitung wurde gefolgt, %d Aufrufe", besucht)
	}
}

// Die Antwort des Prometheus wird nach dem Rechnernamen sortiert, und der Wert
// steht in value[1] als Zeichenkette.
func TestPrometheusFragen(t *testing.T) {
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "node_uname_info" {
			t.Errorf("unerwartete Abfrage: %q", r.URL.Query().Get("query"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
		  {"metric":{"__name__":"node_uname_info","instance":"10.0.0.5:9100","release":"6.1.0-38","sysname":"Linux"},
		   "value":[1788000000,"1"]}]}}`))
	}))
	defer dienst.Close()

	nach := prometheusFragen(context.Background(), dienst.URL, "node_uname_info")
	reihe, ok := nach["10.0.0.5"]
	if !ok {
		t.Fatalf("Rechner nicht gefunden, bekam %v", nach)
	}
	if reihe.Labels["release"] != "6.1.0-38" || reihe.Wert != "1" {
		t.Fatalf("falsch gelesen: %+v", reihe)
	}
}

// Antwortet der Prometheus nicht, bleibt die Spalte leer und die Uebersicht
// steht trotzdem. Eine Nebenquelle darf die Hauptaussage nicht mitreissen.
func TestPrometheusStilleStoertNicht(t *testing.T) {
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dienst.Close()

	if nach := prometheusFragen(context.Background(), dienst.URL, "node_os_info"); nach != nil {
		t.Fatalf("aus einem Fehler wurde eine Antwort: %v", nach)
	}
}
