package de.jonasgroll.nexora;

import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Bundle;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;

import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeActivity;
import com.getcapacitor.BridgeWebViewClient;

import java.util.HashSet;
import java.util.Set;

/**
 * Nexora auf dem Telefon.
 *
 * Die App startet auf einer kleinen Seite in der App (web/index.html), auf der
 * man den Server waehlt. Danach zeigt das Fenster den Server selbst; offline
 * arbeitet dessen Service Worker weiter (public/sw.js im Frontend).
 *
 * Im Fenster bleiben nur: die Startseite, der gewaehlte Server und der
 * Anmeldeanbieter (SSO), zu dem dieser Server beim Anmelden weiterleitet.
 * Alles andere oeffnet im Browser, damit keine fremde Seite im Fenster stehen
 * kann, die wie Nexora aussieht.
 */
public class MainActivity extends BridgeActivity {

    private static final String EINSTELLUNGEN = "nexora";
    private volatile boolean aufStartseite = true;
    private String letzteAdresse = "";

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Bridge b = getBridge();
        b.setWebViewClient(new Leitstelle(b));
        b.getWebView().addJavascriptInterface(new Bruecke(), "NexoraApp");
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(EINSTELLUNGEN, MODE_PRIVATE);
    }

    private String server() {
        return prefs().getString("server", "");
    }

    private static String ursprung(Uri u) {
        if (u == null || u.getScheme() == null || u.getHost() == null) return "";
        String s = u.getScheme() + "://" + u.getHost();
        if (u.getPort() != -1) s += ":" + u.getPort();
        return s;
    }

    private Set<String> anbieter() {
        return prefs().getStringSet("anbieter:" + server(), new HashSet<>());
    }

    /** Die Startseite fragt und setzt hierueber den Server. */
    private class Bruecke {
        @JavascriptInterface
        public String server() {
            return aufStartseite ? MainActivity.this.server() : "";
        }

        @JavascriptInterface
        public boolean setzen(String adresse) {
            // Nur die eigene Startseite darf den Server bestimmen, keine Seite
            // eines Servers.
            if (!aufStartseite) return false;
            Uri u = Uri.parse(adresse);
            if (!"https".equals(u.getScheme()) && !"http".equals(u.getScheme())) return false;
            String neu = ursprung(u);
            if (neu.isEmpty()) return false;
            prefs().edit().putString("server", neu).apply();
            return true;
        }
    }

    private class Leitstelle extends BridgeWebViewClient {
        private final Bridge bridge;

        Leitstelle(Bridge bridge) {
            super(bridge);
            this.bridge = bridge;
        }

        @Override
        public void onPageStarted(WebView view, String url, android.graphics.Bitmap favicon) {
            aufStartseite = url != null && ursprung(Uri.parse(url)).equals(ursprung(Uri.parse(bridge.getLocalUrl())));
            super.onPageStarted(view, url, favicon);
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            Uri ziel = request.getUrl();
            String zielUrsprung = ursprung(ziel);
            String von = letzteAdresse;
            letzteAdresse = ziel.toString();

            if (zielUrsprung.equals(ursprung(Uri.parse(bridge.getLocalUrl())))) {
                return super.shouldOverrideUrlLoading(view, request);
            }
            String s = server();
            if (!s.isEmpty() && zielUrsprung.equals(s)) return false;
            if (anbieter().contains(zielUrsprung) && "https".equals(ziel.getScheme())) return false;

            // Der eine Sprung, der einen Anbieter lehrt: die Weiterleitung, die
            // der Server selbst auf /api/auth/oidc/start ausgibt.
            Uri vonUri = Uri.parse(von);
            if (request.isRedirect()
                    && "https".equals(ziel.getScheme())
                    && !s.isEmpty()
                    && ursprung(vonUri).equals(s)
                    && "/api/auth/oidc/start".equals(vonUri.getPath())) {
                Set<String> neu = new HashSet<>(anbieter());
                neu.add(zielUrsprung);
                prefs().edit().putStringSet("anbieter:" + s, neu).apply();
                return false;
            }

            String schema = ziel.getScheme();
            if ("https".equals(schema) || "http".equals(schema) || "mailto".equals(schema)) {
                try {
                    startActivity(new Intent(Intent.ACTION_VIEW, ziel));
                } catch (Exception ignored) {
                    // kein Browser: dann eben nichts
                }
            }
            return true;
        }
    }
}
