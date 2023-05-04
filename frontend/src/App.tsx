import React from "react";
import { Homepage } from "./pages/Homepage";
import { NoPage } from "./pages/NoPage";
import { GET, POST } from "./backendApi";
import { Product } from "./productcatalog";
import { BrowserRouter, Routes, Route, useNavigate } from "react-router-dom";
import { LoginPage, RegisterPage } from "./pages";
import { AppNavBar } from "./tetris/AppNavBar";
import { Footer } from "./tetris/Footer";
import { redirect } from "react-router-dom";
import CartProvider from "./cart/useCart";

function App() {
  const [products, setProducts] = React.useState<Product[]>([]);
  const [sessionID, setSessionID] = React.useState<string>("");

  React.useEffect(() => {
    GET("/products").then((res) => res.json()).then((resp) => {
      setProducts(resp.data);
    });
  }, []);

  const onLogin = function (email: string, password: string, rememberMe: boolean): void {
    POST("/auth/login", {
        body: JSON.stringify({
            username: email,
            password: password
        }),
    }).then(resp => resp.json()).then((resp) => {
      console.log(resp.data.session_id);
         if (resp.data.session_id !== "") {
            localStorage.setItem("sessionID", resp.data.session_id);
            setSessionID(resp.data.sessionID);
            return redirect("/");
         }
    })
  }

  const onSignUp = function (email: string, password: string): void {
    POST("/auth/register", {
        body: JSON.stringify({
            username: email,
            password: password
        }),
    }).then((res) => {
        console.log(res);
    })
  }

  return (
    <>
      <BrowserRouter>
      <CartProvider>
          <AppNavBar loggedIn={sessionID !== ""} />
          <Routes>
            <Route path="/" element={<Homepage products={products} loggedIn={sessionID !== ""} />} />
            <Route path="/cart" />
            <Route path="/login" element={<LoginPage onLogin={onLogin}  />} />
            <Route path="/register" element={<RegisterPage onSignUp={onSignUp}  />} />
            <Route path="*" element={<NoPage />} />
          </Routes>
        </CartProvider>
        <Footer />
      </BrowserRouter>
    </>
  );
}

export default App;
