import { RESTDataSource } from '@apollo/datasource-rest';

interface Product {
    id: number;
    name: string;
}

export class ProductsAPI extends RESTDataSource {
  override baseURL = 'http://localhost:8080/api/v1/';

  async featuredProducts(): Promise<Product[]> {
    const data = await this.get('products');
    return data.data;
  }
  async product(id: string): Promise<Product> {
    const data = await this.get('product/'+id);
    return data.data;
  }
}
