# https://docs.appoptics.com/kb/apm_tracing/python/install/#flask-and-generic-wsgi
from flask import Flask
from appoptics.middleware import AppOpticsMiddleware

app = Flask(__name__)
app.wsgi_app = AppOpticsMiddleware(app.wsgi_app)

@app.route("/")
def hello():
    return "Hello from Flask!"
