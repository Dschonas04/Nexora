// Wie groß darf eine Übertragung wirklich sein.
//
// Die Einstellung max_anhang_mb ist nur die letzte von mehreren Grenzen. Davor
// liegt in der Regel mindestens ein nginx, oft zwei: der im Frontend-Abbild und
// der Reverse-Proxy davor. Ist deren client_max_body_size kleiner, bricht die
// Übertragung ab, bevor Nexora sie überhaupt sieht. Der eingestellte Wert steht
// dann da und stimmt nicht, und der Fehler, den der Benutzer sieht, kommt von
// einem Dienst, von dem er nichts weiß.
//
// Ausrechnen lässt sich das nicht: Nexora kennt die Konfiguration der Dienste
// vor ihm nicht und soll sie auch nicht kennen, dafür bräuchte es Zugriff auf
// deren Wirt. Messen lässt es sich aber. Dieser Weg nimmt einen Rumpf entgegen
// und wirft ihn weg; wie weit er kommt, sagt der Browser, der ihn geschickt
// hat. Gemessen wird damit genau die Strecke, auf der es später schiefgeht:
// vom Browser durch alles, was dazwischen steht, bis hierher.
package handlers

import (
	"io"
	"net/http"

	"nexora/internal/middleware"
)

// So viel nimmt der Weg höchstens an. Nicht als Schutz vor einer großen Datei,
// die wird ohnehin nur gezählt und weggeworfen, sondern damit niemand die
// Leitung darüber beliebig lange belegen kann.
const grenzprobeMax = 512 << 20

func (s *Server) Grenzprobe(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	// Gezählt und verworfen. Der Inhalt ist gleichgültig, es geht allein darum,
	// ob er ankommt; ihn irgendwo abzulegen wäre eine Einladung, diesen Weg als
	// Ablage zu missbrauchen.
	n, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, grenzprobeMax))
	if err != nil {
		// MaxBytesReader hat den Rumpf abgeschnitten, oder die Verbindung ist
		// unterwegs gefallen. Beides ist für den Messenden dasselbe Ergebnis:
		// so viel geht nicht.
		writeErr(w, http.StatusRequestEntityTooLarge, "zu groß")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"bytes": n})
}
