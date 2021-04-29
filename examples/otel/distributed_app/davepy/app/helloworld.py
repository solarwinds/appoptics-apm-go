# https://docs.appoptics.com/kb/apm_tracing/python/install/#flask-and-generic-wsgi
from flask import Flask
from appoptics.middleware import AppOpticsMiddleware

application = Flask(__name__)
application.wsgi_app = AppOpticsMiddleware(application.wsgi_app)

@application.route("/")
def hello():
    return "Hello from Flask!"
