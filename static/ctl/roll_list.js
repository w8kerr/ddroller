var ddrollerApp = angular.module("ddrollerApp", []);

ddrollerApp.controller("RollListController", function RollListController($scope, $http) {
    var recordsPerPage = 20;
    var mostRecentSeqID = 0;

    $scope.records = [];
    
    function checkForUpdates() {
        getRecordsSince(mostRecentSeqID).then(function(response) {
            var newRecords = response.data;
            if(newRecords) {
                populateRecords(newRecords);
                updateSeqID($scope.records);
            }
        });
    }

    function updateSeqID(records) {
        mostRecentSeqID = 0;
        for(i in records) {
            if(records[i].SeqID > mostRecentSeqID) {
                mostRecentSeqID = records[i].SeqID;
            }
        }
    }

    function getRecordsSince(SeqID) {
        var request = "/rolls.json"
        if(SeqID) {
            request = request + "?since=" + SeqID;
        }
        return $http.get(request);
    }

    function populateRecords(newRecords) {
        newRecords = newRecords.concat($scope.records);
        //Sort in descending order based on SeqID
        newRecords.sort(function(a, b) { return b.SeqID - a.SeqID; });
        //Delete all except the first [recordsPerPage] elements
        newRecords.splice(recordsPerPage, newRecords.length - recordsPerPage);
        $scope.records = newRecords;
    }

    getRecordsSince().then(function(response) {
        var startingRecords = response.data;
        if(startingRecords) {
            populateRecords(startingRecords);
            updateSeqID($scope.records);
        }
    });

    //Poll server for updates every second
    setInterval(checkForUpdates, 1000)
});

