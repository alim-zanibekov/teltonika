#! /usr/bin/env python3

from html.parser import HTMLParser
from operator import le
from urllib import request
import json
import sys

INCLUDE_GROUPS = set(json.loads(sys.argv[1]))

def isInt(s):
    try:
        int(s)
        return True
    except ValueError:
        return False

class Parser(HTMLParser):
  def __init__(self):
    super().__init__()
    self.in_cell = False
    self.cell_index = -1
    self.cell_cache = []
    self.row_cache = []
    self.table = []

  def handle_starttag(self, tag, attrs):
    if tag == 'tr':
      self.cell_index = -1
      if len(self.row_cache) > 0 and isInt(self.row_cache[0]):
        self.table.append(self.row_cache)
      self.row_cache = []
    if tag == 'td':
      self.in_cell = True
      self.cell_cache = []
      self.cell_index += 1

  def handle_endtag(self, tag):
    if tag == 'td':
      self.in_cell = False
      self.row_cache.append('\n'.join(self.cell_cache))

  def handle_data(self, data):
    if self.in_cell:
      s = data.strip()
      if len(s) > 0:
        self.cell_cache.append(s)

fp = request.urlopen("https://wiki.teltonika-mobility.com/view/Full_AVL_ID_List")

pageData = fp.read().decode("utf8")

fp.close()

parser = Parser()
parser.feed(pageData)
table = parser.table

serializedTable = ""

for element in table:
  type = element[3]
  if element[10] not in INCLUDE_GROUPS:
    continue
  line = '    {{{}, "{}", {}, {}, {}, {}, {}, "{}", "{}", "{}", "{}"}},'.format(
    element[0],
    element[1],
    element[2] if isInt(element[2]) else 0,
    ("AVLTypeUnsigned" if type == "Unsigned" else 
    ("AVLTypeSigned" if type == "Signed" else 
    ("AVLTypeHEX" if type == "HEX" else
    ("AVLTypeASCII" if type == "ASCII" else "")))),
    element[4] if len(element[4]) > 0 and element[4] != '-' else 0,
    element[5] if len(element[5]) > 0 and element[5] != '-' else 0,
    float(element[6]) if len(element[6]) > 0 and element[6] != '-' else 0,
    element[7].replace('"', '\\"') if len(element[7]) > 0 and element[7] != '-' else "",
    element[8].replace('"', '\\"') if len(element[8]) > 0 and element[8] != '-' else "",
    ', '.join(element[9].split('\n')),
    element[10]
  )

  serializedTable += line.replace('\n', '\\n') + '\n'
  
print("""package ioelements

var elements = []Info{
""" + serializedTable + """}
""")
