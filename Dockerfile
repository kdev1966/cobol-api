#  Dockerfile

# --- Compilation du programme COBOL -----------------------------------------
FROM debian:12 AS cobol-builder

ENV DEBIAN_FRONTEND=noninteractive

# GnuCOBOL traduit en C : le paquet tire le compilateur dont il a besoin.
RUN apt-get update && \
    apt-get install -y --no-install-recommends gnucobol \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY cobol/ ./cobol/

# Compilation suivie d'un test de fumee sur un cas de reference : le build
# echoue si la mensualite ou le nombre de lignes changent.
RUN mkdir -p /out && \
    cobc -x -free cobol/loan-amortization.cbl -o /out/loan_amortization && \
    echo "0000025000000034500000240A" | /out/loan_amortization > /tmp/fumee.txt && \
    grep -q '^R02400000000144348' /tmp/fumee.txt && \
    test "$(wc -l < /tmp/fumee.txt)" -eq 241

# --- Compilation du service Go ----------------------------------------------
FROM golang:1.26-bookworm AS go-builder

# libcob permet d'executer le binaire COBOL pendant les tests.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y --no-install-recommends libcob4 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Dependances avant les sources : la couche reste en cache tant que les
# manifestes ne bougent pas.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=cobol-builder /out/loan_amortization ./bin/loan_amortization

# La suite de tests tourne a la construction, invariants de l'echeancier
# compris : une regression bloque l'image.
RUN go vet ./... && go test ./...

# CGO desactive : le binaire est statique et ne depend pas de la libc de
# l'image d'execution.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cobol-api ./cmd/cobol-api

# --- Execution : ni compilateur C, ni compilateur COBOL, ni chaine Go -------
FROM debian:12-slim

ENV DEBIAN_FRONTEND=noninteractive

# libcob4 suffit a executer le programme COBOL ; curl sert au HEALTHCHECK.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libcob4 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=cobol-builder /out/loan_amortization ./bin/loan_amortization
COPY --from=go-builder /out/cobol-api ./cobol-api

# Utilisateur non privilegie. Le service n'ecrit rien sur disque.
RUN useradd --system --create-home --shell /usr/sbin/nologin cobolapi
USER cobolapi

ENV COBOL_PROGRAM_PATH=/app/bin/loan_amortization \
    APP_ENV=production \
    PORT=3000

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:3000/health || exit 1

CMD ["/app/cobol-api"]
