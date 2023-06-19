import { RESTDataSource } from '@apollo/datasource-rest';

interface Cart {
    items: CartItem[];
}

interface CartItem {
    id: string;
    name: string;
}

export class CartDS extends RESTDataSource {
  override baseURL = 'http://localhost:8080/api/v1/';

  async cart(id: string): Promise<Cart> {
    const data = await this.get('cart/'+id);
    return data.data;
  }

  async addToCart(cartId: string, productId: string, quantity: number): Promise<string> {
    const data = await this.post('cart/'+cartId+'/item', {
      productId,
      quantity
    });
    return data.data;
  }
}
