# webrt-network-mapper — build și instalare globală
APP      := webrt-network-mapper
PREFIX   ?= /usr/local
BINDIR   := $(PREFIX)/bin
VERSION  ?= 1.0.0
LDFLAGS  := -s -w -X $(APP)/internal/api.Version=$(VERSION)
USERBIN  := $(HOME)/.local/bin
APPS_DIR := $(HOME)/.local/share/applications
ICON_DIR := $(HOME)/.local/share/icons/hicolor/scalable/apps

.PHONY: build run test install install-user uninstall uninstall-user desktop dist clean

build: ## compilează binarul în folderul curent
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) ./cmd/$(APP)

run: build ## pornește interfața web
	./$(APP) web

test:
	go vet ./...
	go test ./...

install: build ## instalare globală în /usr/local/bin (cere sudo) + ARP raw fără root
	sudo install -m 0755 $(APP) $(BINDIR)/$(APP)
	@if command -v setcap >/dev/null; then \
		sudo setcap cap_net_raw+ep $(BINDIR)/$(APP) && echo "ARP raw activat (cap_net_raw)"; \
	else echo "setcap lipsește (sudo apt install libcap2-bin) — se va folosi ping + tabela ARP"; fi
	@$(MAKE) --no-print-directory desktop
	@echo "Instalat: $(BINDIR)/$(APP)  →  rulează: $(APP)"

install-user: build ## instalare fără sudo în ~/.local/bin (fără ARP raw)
	install -D -m 0755 $(APP) $(USERBIN)/$(APP)
	@$(MAKE) --no-print-directory desktop
	@echo "Instalat: $(USERBIN)/$(APP)"
	@case ":$$PATH:" in *":$(USERBIN):"*) ;; *) echo "Atenție: $(USERBIN) nu e în PATH. Adaugă în ~/.bashrc:  export PATH=\"\$$HOME/.local/bin:\$$PATH\"";; esac

desktop: ## lansator în meniul de aplicații (Linux)
	@if [ "$$(uname)" = "Linux" ]; then \
		install -D -m 0644 packaging/$(APP).desktop $(APPS_DIR)/$(APP).desktop; \
		install -D -m 0644 packaging/$(APP).svg $(ICON_DIR)/$(APP).svg; \
		command -v update-desktop-database >/dev/null && update-desktop-database $(APPS_DIR) >/dev/null 2>&1 || true; \
		echo "Lansator adăugat în meniul de aplicații: Network Mapper"; \
	fi

uninstall: ## dezinstalează versiunea globală (datele rămân)
	sudo rm -f $(BINDIR)/$(APP)
	rm -f $(APPS_DIR)/$(APP).desktop $(ICON_DIR)/$(APP).svg
	@echo "Dezinstalat. Datele au rămas în ~/.local/share/$(APP) și instantaneele în ~/Documente/$(APP)."

uninstall-user:
	rm -f $(USERBIN)/$(APP) $(APPS_DIR)/$(APP).desktop $(ICON_DIR)/$(APP).svg

dist: ## binare pentru Linux, Windows și macOS în dist/
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-linux-amd64       ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-linux-arm64       ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-windows-amd64.exe ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-macos-arm64       ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-macos-amd64       ./cmd/$(APP)

clean:
	rm -rf $(APP) dist
