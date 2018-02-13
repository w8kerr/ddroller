var ddrollerApp = angular.module("ddrollerApp", []);

ddrollerApp.controller("RollListController", function RollListController($scope) {
    $scope.records = [
        {
            total: 20,
            SeqID: 1,
        },
        {
            total: 9,
            SeqID: 2,
        },
        {
            total: 4,
            SeqID: 3,
        },
        {
            total: 19,
            SeqID: 4,
        },
    ];
});