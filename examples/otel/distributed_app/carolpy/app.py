from flask import Flask
from appoptics.middleware import AppOpticsMiddleware

application = Flask(__name__)
application.wsgi_app = AppOpticsMiddleware(application.wsgi_app)


@application.route('/carol')
def hello():
    return "Hello from carolpy/app.py"

if __name__ == '__main__':
    application.run()
