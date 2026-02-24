FROM python:3.12-slim

# System tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    bc gawk sed grep curl && \
    rm -rf /var/lib/apt/lists/*

# Python packages for data/file generation
RUN pip install --no-cache-dir \
    pandas \
    numpy \
    matplotlib \
    openpyxl \
    reportlab \
    python-docx \
    pillow

# Pre-import to warm up .pyc caches
RUN python3 -c "import pandas, numpy, matplotlib, openpyxl, reportlab, docx, PIL; print('all imports ok')"

# Default working dirs
RUN mkdir -p /workspace /output /data

WORKDIR /workspace
