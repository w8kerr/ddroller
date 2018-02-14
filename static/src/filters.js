var ddrollerApp = angular.module("ddrollerApp");

ddrollerApp.filter("successshort", function() {
    return function(input) {
        if(input < 0) {
            return "FAIL";
        }
        else if(input > 0) {
            return "SUC";
        }
        else {
            return "RES";
        }
    }
})

ddrollerApp.filter("idtoslug", function() {
    return function(input) {
        var slug = input.toString(36);
        if(slug.length < 4) {
            slug = "0".repeat(4 - slug.length) + slug;
        }
        return slug;
    }
})