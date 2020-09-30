#!/bin/bash
openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
  -keyout for_test.key -out for_test.crt -extensions san -config \
  <(echo "[req]";
    echo distinguished_name=req;
    echo "[san]";
    echo subjectAltName=DNS:localhost
    ) \
  -subj "/CN=localhost"
