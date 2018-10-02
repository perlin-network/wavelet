#!/usr/bin/python
# -*- coding: utf-8 -*-

from flask import jsonify
import json
import string

class InvalidUsage(Exception):
    def __init__(self, message, status_code=400, payload=None):
        Exception.__init__(self)
        self.message = message
        self.status_code = status_code
        self.payload = payload

    def to_dict(self):
        rv = dict(self.payload or ())
        rv['message'] = self.message
        rv['status'] = self.status_code
        return rv

def check_for_fields(content, required):
    for field in required:
        if field not in content:
            raise InvalidUsage("Expected '%s' field in request." % (field))

def return_message(message):
    msg = {
        "status": 200,
        "message": message,
    }
    resp = jsonify(msg)
    return resp

def dumper(obj):
    try:
        return obj.toJSON()
    except:
        return obj.__dict__
