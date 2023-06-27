// chrome.runtime.onMessage.addListener((message) => {
//   if (message.action === "showAlert") {
//     chrome.action.getPopup({}, (result) => {
//       chrome.action.setPopup({ tabId: result.tabId, popup: "popup.html?alertMessage=" + message.message });
//     });
//   }
// });


chrome.runtime.onMessage.addListener(function(request, sender, sendResponse) {
  if (request.action === "createNotification") {
    chrome.notifications.create({
      type: "basic",
      iconUrl: "icon38.png",
      title: "Notification Title",
      message: "Notification Message"
    }, function(notificationId) {
      console.log("Notification created with ID: " + notificationId);
    });
  }
});
