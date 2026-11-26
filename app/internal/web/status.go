package web

import (
	"encoding/json"
	"net/http"
)

type verificationView struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action"`
	Button  string `json:"button"`
	Poll    bool   `json:"poll"`
}

func viewVerification(status string, enabled bool) verificationView {
	v := verificationView{Status: status, Action: "none"}
	switch status {
	case "not_started":
		v.Message = "Проверка не начата"
		v.Button = "Подтвердить личность"
	case "in_progress":
		v.Message = "Продолжите проверку"
		v.Button = "Продолжить проверку"
		v.Poll = true
	case "pending":
		v.Message = "Материалы отправлены, ожидаем результат"
		v.Poll = true
	case "on_hold":
		v.Message = "Проверка требует дополнительного рассмотрения"
		v.Poll = true
	case "approved":
		v.Message = "Личность подтверждена"
		v.Action = "meme"
	case "retry":
		v.Message = "Нужно исправить материалы"
		v.Button = "Исправить материалы"
		v.Poll = true
	case "rejected":
		v.Message = "Проверка отклонена"
	default:
		v.Message = "Статус проверки уточняется"
		v.Poll = true
	}
	if v.Button != "" && enabled {
		v.Action = "sdk"
	}
	return v
}

func tokenAllowed(status string) bool {
	switch status {
	case "not_started", "in_progress", "pending", "on_hold", "retry":
		return true
	}
	return false
}

func (h *handler) verificationStatus(w http.ResponseWriter, r *http.Request) {
	user := h.verificationUser(w, r)
	if user == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(viewVerification(user.Status, h.issuer != nil))
}
