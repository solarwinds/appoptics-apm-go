from flask import Flask
from oboeware import OboeMiddleware
import oboe

oboe.config['tracing_mode'] = 'always'
application = Flask(__name__)
application.wsgi_app = OboeMiddleware(application.wsgi_app)

@application.route("/")
def hello():
    return "Hello from Flask!"
