import sys, getopt
import random
import json
import time
import signal

from requests_toolbelt import MultipartEncoder
from common import rest_api_client, log
from datetime import datetime

logger = log.MyLog()

TYPE_INSERT = '--insert'
TYPE_DUMP = '--dump'

def signal_handler(sig, frame):
    sys.exit(0)

class StatefulAppUtils:

    def __init__(self, url):
        self.url = url
        self.session = rest_api_client.RestClient(url).session

    def db_insert(self, name):
        dateTimeObj = datetime.now()
        timestampStr = dateTimeObj.strftime("%d-%b-%Y (%H:%M:%S.%f)")
        name = name + timestampStr 
        age = random.randint(10, 50)
        user_info = {
            "name": name,
            "age": age
        }
        result = self.session.post(self.url + "/user/add", json=user_info)
        return json.loads(result.text)

    def db_query_all(self):
        result = self.session.get(self.url + "/user/all")
        return json.loads(result.text)

    def db_query(self, user_id):
        result = self.session.get(self.url + "/user/" + str(user_id))
        # print(result.text)
        return json.loads(result.text)

    def db_delete(self, user_id):
        result = self.session.post(self.url + "/user/delete/" + str(user_id))
        # print(result.text)
        return json.loads(result.text)


if __name__ == '__main__':
    signal.signal(signal.SIGINT, signal_handler)

    type = ''
    if len(sys.argv) != 2:
        print('db-generator.py', TYPE_INSERT, '|', TYPE_DUMP)
        sys.exit(1)

    if sys.argv[1] == TYPE_INSERT:
        type = TYPE_INSERT
    elif sys.argv[1] == TYPE_DUMP:
        type = TYPE_DUMP
    else:
        print('db-generator.py', TYPE_INSERT, '|', TYPE_DUMP)
        sys.exit(1)

    stateful_app_utils = StatefulAppUtils("http://127.0.0.1:30176/jibu")
    if type == TYPE_DUMP:
        result = stateful_app_utils.db_query_all()
        #print(result)
        for item in result:
            print(item)
        sys.exit(0)

    while True:
        time.sleep(3)
        result = stateful_app_utils.db_insert("test-")
        print('saved db record: ', result)

