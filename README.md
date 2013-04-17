Building
--------
To build for development purposes get a copy of Go from [golang.org](http://golang.org/doc/install)

Once you have go installed then fetching and building ptserver can be done with the following command:

go get -u github.com/placetime/ptserver

Go will fetch all dependencies automatically and build the server for you/ You may have to supply github credentials more than once to access the placetime private repositories.


Live Deployment
---------------
To deploy just copy the ptserver binary to the right location and run. See the [configuration](http:/github.com/placetime/configuration) repository for init scripts.
