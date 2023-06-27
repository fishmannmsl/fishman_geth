$(function () {
  $("#loadPK").click(function () {
    var privatekeyvalue = $("#inputPrivateKey").val()
    console.log(privatekeyvalue)
    let data = {
      privatekey: privatekeyvalue
    }
    $.ajax({
      url: "http://127.0.0.1:3000/getwallet",
      type: "POST",
      contentType: "application/json",
      data: JSON.stringify(data),
      success: function (response) {
        $("#inputPrivateKey").val(response["Privatekey"]);
        $("#inputPublic").val(response["Publickey"]);
        $("#inputAddress").val(response["Address"]);
        saveUserInfo()
        getUserAmount()
        console.info(response);
      },
      error: function (error) {
        console.error(error);
      },
    });
  })
  $("#randGe").click(function () {
    var privatekeyvalue = $("#inputPrivateKey").val()
    console.log(privatekeyvalue)
    $.ajax({
      url: "http://127.0.0.1:3000/addwallet",
      type: "POST",
      success: function (response) {
        $("#inputPrivateKey").val(response["Privatekey"]);
        $("#inputPublic").val(response["Publickey"]);
        $("#inputAddress").val(response["Address"]);
        saveUserInfo()
        getUserAmount()
        console.info(response);
      },
      error: function (error) {
        console.error(error);
      },
    });
  })
  $("button.transList").click(function () {
    storage.get('data', function (result) {
      // 检查是否存在之前保存的数据
      if (result.data) {
        $.ajax({
          url: "http://localhost:3000/gethistory",
          type: "GET",
          contentType: "application/json",
          success: function (response) {
            var template = $("#transactionTemplate").html();
            var r = Mustache.render(template, {
              transactions: response["Transactions"]
            })
            $("#transData").html(r)
            console.info(response);
          },
          error: function (error) {
            console.error(error);
          },
        });
      }
    });
  })
  $("#buttonSubmit").click(function () {
    let confirm_text = "确定要发送吗?";
    let confirm_result = confirm(confirm_text);
    if (confirm_result !== true) {
      alert("取消");
      return;
    }
    let transaction_data = {
      sender_blockchain_address: $("#inputAddress").val(),
      recipient_blockchain_address: $("#inputReceiveAddress").val(),
      value: $("#inputAmount").val(),
    };
    $.ajax({
      url: "http://localhost:3000/sendtransation",
      type: "POST",
      contentType: "application/json",
      data: JSON.stringify(transaction_data),
      success: function (response) {
        console.info("response.message:", response.message);
        if (response.message === "fail") {
          alert("交易失败");
          return;
        }
      },
      error: function (response) {
        console.error(response);
        alert("发送失败");
      },
    });
  });
  $("#serachSubmit").click(function () {
    var inputSearchBlock = $("#inputSearchBlock").val()
    var inputSearchTran = $("#inputSearchTran").val()
    var isPureNumber = !isNaN(parseFloat(inputSearchBlock)) && isFinite(inputSearchBlock)
    console.log(inputSearchBlock)
    if (inputSearchBlock == "" && inputSearchTran == "") {
      alert("请输入数据")
    }else if (inputSearchBlock != "" && inputSearchTran != "") {
      alert("只能同时查询一个")
      return
    } else if (inputSearchBlock != "") {
      // if (!isPureNumber) {
      //   searchBlockByNumber(inputSearchBlock);
      // } else {
      //   searchBlockByHash(inputSearchBlock);
      // }
      searchBlockByNumber(inputSearchBlock);
    } else if (inputSearchTran != "") {
      searchTransactionByHash(inputSearchTran);
    }
  });


})


var storage = chrome.storage.sync;
function saveUserInfo() {
  // 获取用户输入的数据
  var inputData = document.getElementById('inputPrivateKey').value;
  var publicKey = document.getElementById('inputPublic').value;
  var address = document.getElementById('inputAddress').value;
  let data = {
    privateKey: inputData,
    publicKey: publicKey,
    address: address,
  }
  // 将数据保存到存储中
  storage.set({ 'data': data }, function () {
    console.log('数据已保存');
  });
};

document.addEventListener('DOMContentLoaded', function () {
  // 从存储中检索数据
  getUserData()
  getUserAmount()
});

function getUserData() {
  storage.get('data', function (result) {
    // 检查是否存在之前保存的数据
    if (result.data) {
      // 将数据应用到用户界面
      document.getElementById('inputPrivateKey').value = result.data.privateKey;
      document.getElementById('inputPublic').value = result.data.publicKey;
      document.getElementById('inputAddress').value = result.data.address;
    }
  });
}

function searchBlockByHash(inputHash) {
  let _postdata = {
    hash: inputHash
  };
  $.ajax({
    url: "http://127.0.0.1:3000/blockbyhash",
    type: "POST",
    contentType: "application/json",
    data: JSON.stringify(_postdata),
    success: function (response) {
      var template = $("#blockTemplate").html();
      var r = Mustache.render(template, {
        timestamp: response["Timestamp"],
        nonce: response["Nonce"],
        hash: response["Hash"],
        number: response["Number"],
        difficulty: response["Difficulty"],
        reward:response["Reward"],
        coinbase:response["Coinbase"],
        previous_hash: response["PrevHash"],
        transactions: response["Transactions"]
      })
      $("#searchResult").html(r)
      console.info(response);
    },
    error: function (error) {
      console.error(error);
    },
  });
}

function searchBlockByNumber(inputNumber) {
  let _postdata = {
    number: inputNumber
  };
  $.ajax({
    url: "http://127.0.0.1:3000/blockbynumber",
    type: "POST",
    contentType: "application/json",
    data: JSON.stringify(_postdata),
    success: function (response) {
      var template = $("#blockTemplate").html();
      var r = Mustache.render(template, {
        timestamp: response["Timestamp"],
        nonce: response["Nonce"],
        hash: response["Hash"],
        number: response["Number"],
        difficulty: response["Difficulty"],
        reward:response["Reward"],
        coinbase:response["Coinbase"],
        previous_hash: response["PrevHash"],
        transactions: response["Transactions"]
      })
      $("#searchResult").html(r)
      console.info(response);
    },
    error: function (error) {
      console.error(error);
    },
  });
}

function searchTransactionByHash(inputHash) {
  let _postdata = {
    hash: inputHash
  };
  $.ajax({
    url: "http://127.0.0.1:3000/transationtohash",
    type: "POST",
    contentType: "application/json",
    data: JSON.stringify(_postdata),
    success: function (response) {
      var template = $("#transactionTemplate").html();
      var r = Mustache.render(template, {
        transactions: [response]
      })
      $("#searchResult").html(r)
      console.info(response);
    },
    error: function (error) {
      console.error(error);
    },
  });
}

function getUserAmount() {
  storage.get('data', function (result) {
    // 检查是否存在之前保存的数据
    if (result.data) {
      let _postdata = {
        blockchain_address: result.data.address
      };
      console.log("blockaddress:", JSON.stringify(_postdata));
      $.ajax({
        url: "http://127.0.0.1:3000/getbalance",
        type: "POST",
        contentType: "application/json",
        data: JSON.stringify(_postdata),
        success: function (response) {
          $("#wallet_amount").text(response["balance"]);
          console.info(response);
        },
        error: function (error) {
          console.error(error);
        },
      });
    }
  });
}
