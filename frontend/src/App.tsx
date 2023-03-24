import React from "react";
import { Homepage } from "./pages/Homepage";
import { NoPage } from "./pages/NoPage";
import { GET } from "./backendApi";
import { Product } from "./productcatalog";
import { BrowserRouter, Routes, Route } from "react-router-dom";

function App() {
  const [products, setProducts] = React.useState<Product[]>([]);

  React.useEffect(() => {
    GET("/products").then((res) => res.json()).then((resp) => {
      setProducts(resp.data);
    });
  }, []);
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Homepage products={products} />} />
        <Route path="/cart" />
        <Route path="*" element={<NoPage />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
