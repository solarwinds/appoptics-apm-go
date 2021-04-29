cd /carolpy
uwsgi --socket 0.0.0.0:8082 --protocol=http -w app
