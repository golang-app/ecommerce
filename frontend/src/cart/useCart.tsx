import { useState, useEffect } from "react";
import { GET, POST } from "../backendApi";
import { Cart } from "./Cart";

export function useCart() {
  const [cart, setCart] = useState<Cart>({ cartID: "", products: [] });

  useEffect(() => {
    let c = cart;
    if (!c?.cartID) {
      // read cartID from local storage
      let localCart = localStorage.getItem("cart");
      if (localCart) {
        c = JSON.parse(localCart);
      } else {
        // generate new cartID
        let cartID = Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);
        let localCart = {
          cartID,
          products: [],
        }

        localStorage.setItem("cart", JSON.stringify(localCart));
        c = localCart;
      }
    }

    GET("/cart/" + c.cartID).then((res) => res.json()).then((resp: any) => {
      c.products = resp.data.items;
      setCart(c);
    });
    return () => {};
  }, []);

  return {
    products: cart.products,
    countItems(): number {
      let count = 0;
      for (const item of cart.products) {
        count += item.quantity;
      }
      return count;
    },
    addProduct(productID: string, quantity: number): void {
      POST("/cart/" + cart.cartID, {
        body: JSON.stringify({
          product_id: productID,
          quantity,
        }),
      }).then(() => {
        GET("/cart/" + cart.cartID).then((res) => res.json()).then((resp: any) => {
          let c = cart;
          c.products = resp.data.items;
          setCart(c);
        });
      });
    }
  };
}
