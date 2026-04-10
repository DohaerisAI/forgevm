FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# ── System packages + CLI tools ──
RUN apt-get update && apt-get install -y \
    # Core
    python3 python3-pip python3-venv python3-dev \
    curl wget git zip unzip jq vim nano htop \
    net-tools iputils-ping build-essential pkg-config \
    # PDF CLI tools
    poppler-utils qpdf pdftk-java pandoc \
    # OCR
    tesseract-ocr tesseract-ocr-eng \
    # Image/Media
    ffmpeg imagemagick inkscape \
    # LibreOffice
    libreoffice-calc libreoffice-writer libreoffice-impress libreoffice-draw \
    # Java (for tabula-py, pdftk)
    default-jre-headless \
    # C/C++
    gcc g++ \
    # SQLite
    sqlite3 libsqlite3-dev \
    # Graphviz
    graphviz libgraphviz-dev \
    # Image libs (for Pillow, Wand, sharp)
    libmagickwand-dev libfreetype-dev libjpeg-dev libpng-dev libtiff-dev \
    libwebp-dev zlib1g-dev libffi-dev libssl-dev \
    # Chromium (for Playwright)
    chromium-browser \
    # Sound
    libasound2-dev portaudio19-dev \
    && rm -rf /var/lib/apt/lists/*

# ── Rust/Cargo ──
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# ── Node.js 22.x ──
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# ── Python packages (--ignore-installed to override deb-managed packages) ──
RUN pip3 install --no-cache-dir --break-system-packages --ignore-installed \
    # Data / Science
    pandas==3.0.1 \
    scipy \
    scikit-learn \
    scikit-image \
    # Visualization
    matplotlib \
    seaborn \
    # Excel / Office
    openpyxl==3.1.5 \
    xlsxwriter \
    python-docx \
    python-pptx==1.0.2 \
    odfpy \
    # PDF
    pypdf==5.9.0 \
    pypdfium2==5.6.0 \
    pdfplumber==0.11.9 \
    pdfminer.six \
    pdf2image==1.17.0 \
    pdfkit \
    pikepdf \
    img2pdf \
    reportlab==4.4.10 \
    camelot-py[cv] \
    tabula-py \
    # Web / Scraping
    flask \
    requests \
    beautifulsoup4 \
    playwright \
    grip \
    # CV / ML
    opencv-python-headless \
    mediapipe \
    onnxruntime \
    flatbuffers \
    # Image
    Pillow==12.1.1 \
    Wand \
    imageio \
    tifffile \
    freetype-py \
    # OCR
    pytesseract==0.3.13 \
    # Markdown / Docs
    markitdown==0.1.5 \
    markdown \
    mkdocs \
    mkdocs-material \
    mistune \
    # Utils
    lxml \
    sympy \
    networkx \
    graphviz \
    psutil \
    cryptography \
    sounddevice \
    # Misc
    Jinja2 \
    PyYAML \
    click \
    tabulate

# Install Playwright browsers
RUN playwright install chromium

# ── Node.js global packages ──
RUN npm install -g \
    # Documents
    docx@9.6.1 \
    pptxgenjs@4.0.1 \
    pdf-lib \
    pdfjs-dist \
    # Mermaid
    @mermaid-js/mermaid-cli \
    # Markdown
    markdown-pdf \
    marked \
    markdown-toc \
    markdownlint-cli \
    markdownlint-cli2 \
    remark-cli \
    # React
    react@19 \
    react-dom@19 \
    react-icons \
    # Image
    sharp \
    # Browser automation
    playwright \
    # TypeScript
    typescript@6 \
    ts-node \
    tsx \
    # Graph
    graphviz

# Make global npm packages resolvable by require()
ENV NODE_PATH=/usr/lib/node_modules

WORKDIR /workspace
