`elements_dump.py` - parses IO Elements description
from [wiki page](https://wiki.teltonika-mobility.com/view/Full_AVL_ID_List).
The only positional argument takes an array of strings in json format,
which lists the 'Parameter Group' from the table to be included in the output file.
On the output (stdout) generates a .go code with a list of IO Element.

You can generate the list yourself and use it via the `ioelements`
package by passing it to the NewParser function

`generate.sh` - Generates a IO Elements list via `elements_dump.py`
with a specific 'Parameter Group' list and updates `../ioelements/ioelements_dump.go`