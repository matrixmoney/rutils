# ==============================================================================
# Stage 1: Build the Vite frontend
# ==============================================================================
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ==============================================================================
# Stage 2: Build the Go backend
# ==============================================================================
FROM golang:alpine AS backend-builder
WORKDIR /app
COPY . .
RUN if [ -d "vendor" ]; then \
        CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o server ./cmd/server/main.go; \
    else \
        apk add --no-cache git ca-certificates && \
        go mod download && \
        CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/main.go; \
    fi

# ==============================================================================
# Stage 3: Runtime image — high-fidelity conversion toolchain
# ==============================================================================
FROM alpine:3.20
WORKDIR /app

# ------------------------------------------------------------------------------
# System toolchain for iLovePDF-grade conversions
# ------------------------------------------------------------------------------
# Package groups:
#   • Document fidelity   : libreoffice-* (writer, calc, impress, draw)
#   • PDF post-processing : ghostscript, qpdf
#   • PDF rendering       : poppler-utils (pdftoppm, pdftocairo, pdftotext, pdfinfo)
#   • Image ↔ PDF         : img2pdf (lossless embedding)
#   • JPEG encoding       : mozjpeg (jpegtran, cjpeg) — 15-25% better than libjpeg
#   • PNG encoding        : pngquant (lossy palette), oxipng (lossless re-compress)
#   • WebP encoding       : libwebp-tools (cwebp, dwebp)
#   • Image processing    : imagemagick + imagemagick-pdf (deskew, denoise, resize)
#   • OCR                 : tesseract-ocr + multiple languages
#   • Fonts               : Carlito/Caladea (metric-compatible with Calibri/Cambria)
#                          Liberation fonts (Arial/Times/Courier metric-compatible)
#                          DejaVu + Noto for Unicode coverage
# ------------------------------------------------------------------------------

RUN apk add --no-cache \
    # --- Documents: LibreOffice headless ---
    libreoffice \
    libreoffice-writer \
    libreoffice-calc \
    libreoffice-impress \
    libreoffice-draw \
    # --- PDF toolchain ---
    ghostscript \
    qpdf \
    poppler-utils \
    # --- Image tools ---
    py3-img2pdf \
    libjpeg-turbo-utils \
    jpegoptim \
    pngquant \
    oxipng \
    libwebp-tools \
    imagemagick \
    imagemagick-pdf \
    imagemagick-jpeg \
    imagemagick-webp \
    imagemagick-tiff \
    imagemagick-svg \
    # --- OCR ---
    tesseract-ocr \
    tesseract-ocr-data-eng \
    tesseract-ocr-data-hin \
    tesseract-ocr-data-fra \
    tesseract-ocr-data-deu \
    tesseract-ocr-data-spa \
    # --- Fonts: MS metric-compatible ---
    font-carlito \
    font-liberation \
    font-dejavu \
    font-noto \
    font-noto-cjk \
    font-noto-emoji \
    fontconfig \
    # --- Timezone + utilities ---
    tzdata \
    ca-certificates \
    bash \
    coreutils \
    file \
    && fc-cache -f -v

# ------------------------------------------------------------------------------
# Configure ImageMagick policy to allow PDF read/write
# Alpine's default policy blocks PDF for security reasons. Since we
# explicitly need PDF manipulation (rasterization, PDF↔image), we must
# re-enable it.
# ------------------------------------------------------------------------------
RUN sed -i 's|rights="none" pattern="PDF"|rights="read\|write" pattern="PDF"|g' \
        /etc/ImageMagick-7/policy.xml 2>/dev/null || true \
    && sed -i 's|rights="none" pattern="PDF"|rights="read\|write" pattern="PDF"|g' \
        /etc/ImageMagick-6/policy.xml 2>/dev/null || true

# ------------------------------------------------------------------------------
# LibreOffice hardening: pre-create user profile directory so soffice
# doesn't try to write to a non-existent $HOME on the first invocation.
# ------------------------------------------------------------------------------
ENV HOME=/tmp
ENV SOFFICE_USER_INSTALL=/tmp/soffice-profile
RUN mkdir -p /tmp/soffice-profile

# ------------------------------------------------------------------------------
# Copy runtime artifacts from the build stages
# ------------------------------------------------------------------------------
COPY --from=backend-builder /app/server .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY --from=backend-builder /app/docs ./docs
COPY --from=backend-builder /app/data ./data
RUN mkdir -p /app/data /app/data/nse/data/companies

# ------------------------------------------------------------------------------
# Expose ports
# ------------------------------------------------------------------------------
EXPOSE 3000 8080

# ------------------------------------------------------------------------------
# Healthcheck: verify binary can report readiness
# ------------------------------------------------------------------------------
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/swagger/doc.json >/dev/null 2>&1 || exit 1

# ------------------------------------------------------------------------------
# Entrypoint
# ------------------------------------------------------------------------------
CMD ["./server"]