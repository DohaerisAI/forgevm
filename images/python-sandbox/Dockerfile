FROM ubuntu:24.04

RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-venv \
    curl \
    wget \
    git \
    zip \
    unzip \
    jq \
    vim \
    nano \
    htop \
    net-tools \
    iputils-ping \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages \
    python-docx \
    python-pptx \
    reportlab \
    pandas \
    numpy

WORKDIR /workspace
