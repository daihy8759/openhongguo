Java.perform(function () {
  var RB = Java.use("com.bytedance.retrofit2.client.Request$Builder");
  var R = Java.use("com.bytedance.retrofit2.client.Response");
  var B64 = Java.use("android.util.Base64");
  var BAOS = Java.use("java.io.ByteArrayOutputStream");
  var REQ_PAT = /device_register|multi_video_model|video_model|video_tab|multi_video_detail/i;

  var build = RB.build.overload();
  build.implementation = function () {
    var r = build.call(this);
    try {
      var u = String(r.getUrl());
      if (REQ_PAT.test(u)) {
        console.log("REQ " + r.getMethod() + " " + u);
        var body = r.getBody();
        if (body) {
          var bos = BAOS.$new();
          body.writeTo(bos);
          var arr = bos.toByteArray();
          bos.close();
          console.log("REQBODY " + B64.encodeToString(arr, 2));
        }
        console.log("REQEND");
      }
    } catch (e) {
      console.log("REQERR " + e);
    }
    return r;
  };

  var getUrl = R.getUrl.overload();
  getUrl.implementation = function () {
    var u = getUrl.call(this);
    try {
      if (REQ_PAT.test(u)) {
        var status = this.getStatus();
        var len = -1;
        var body = this.getBody();
        if (body) len = body.length();
        console.log("RESP " + status + " " + len + " " + u);
        if (body && len > 0 && len < 4000) {
          try {
            console.log("RESPBODY " + B64.encodeToString(body.bytes(), 2));
          } catch (e2) {}
        }
      }
    } catch (e) {
      console.log("RESPERR " + e);
    }
    return u;
  };
  console.log("NET_HOOK_READY");
});
