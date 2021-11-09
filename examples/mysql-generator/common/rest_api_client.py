import requests
from common import log
import json as complexjson

logger = log.MyLog()


class RestClient():

    def __init__(self, api_root_url):
        self.api_root_url = api_root_url
        self.session = requests.session()

    def login(self, url, **kwargs):
        result = self.request(url, "GET", **kwargs)
        assert result.ok
        return self


    def get(self, url, **kwargs):
        return self.request(url, "GET", **kwargs)

    def post(self, url, data=None, json=None, **kwargs):
        return self.request(url, "POST", data, json, **kwargs)

    def put(self, url, data=None, **kwargs):
        return self.request(url, "PUT", data, **kwargs)

    def delete(self, url, **kwargs):
        return self.request(url, "DELETE", **kwargs)

    def patch(self, url, data=None, **kwargs):
        return self.request(url, "PATCH", data, **kwargs)

    def request(self, url, method, data=None, json=None, **kwargs):
        url = self.api_root_url + url
        headers = dict(**kwargs).get("headers")
        params = dict(**kwargs).get("params")
        files = dict(**kwargs).get("files")
        cookies = dict(**kwargs).get("cookies")
        self.request_log(url, method, data, json, params, headers, files, cookies)
        if method == "GET":
            return self.session.get(url, **kwargs)
        if method == "POST":
            return self.session.post(url, data, json, **kwargs)
        if method == "PUT":
            if json:
                data = complexjson.dumps(json)
            return self.session.put(url, data, **kwargs)
        if method == "DELETE":
            return self.session.delete(url, **kwargs)
        if method == "PATCH":
            if json:
                data = complexjson.dumps(json)
            return self.session.patch(url, data, **kwargs)

    def request_log(self, url, method, data=None, json=None, params=None, headers=None, files=None, cookies=None, **kwargs):
        logger.info("addr ==>> {}".format(url))
        logger.info("method ==>> {}".format(method))
        if(headers is not None):
            logger.info("headers ==>> {}".format(complexjson.dumps(headers, indent=4, ensure_ascii=False)))
        if (params is not None):
            logger.info("params ==>> {}".format(complexjson.dumps(params, indent=4, ensure_ascii=False)))
        if (data is not None):
            logger.info("data ==>> {}".format(complexjson.dumps(data, indent=4, ensure_ascii=False)))
        if (json is not None):
            logger.info("json ==>> {}".format(complexjson.dumps(json, indent=4, ensure_ascii=False)))
        if (files is not None):
            logger.info("files ==>> {}".format(files))
        if (cookies is not None):
            logger.info("cookies ==>> {}".format(complexjson.dumps(cookies, indent=4, ensure_ascii=False)))

