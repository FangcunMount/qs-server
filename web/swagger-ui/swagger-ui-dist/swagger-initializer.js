window.onload = function() {
  //<editor-fold desc="Changeable Configuration Block">

  const params = new URLSearchParams(window.location.search);
  const customUrl = params.get("url");
  const defaultUrl = "/api/rest/apiserver.yaml";

  const urls = [
    { name: "QS API Server", url: "/api/rest/apiserver.yaml" },
    { name: "QS Collection", url: "/api/rest/collection.yaml" },
  ];

  window.ui = SwaggerUIBundle({
    url: customUrl || defaultUrl,
    urls: urls,
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [
      SwaggerUIBundle.plugins.DownloadUrl
    ],
    layout: "StandaloneLayout"
  });

  //</editor-fold>
};
