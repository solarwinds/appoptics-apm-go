cd /home/app/
uwsgi --socket 0.0.0.0:8083 --protocol=http -w wsgi
